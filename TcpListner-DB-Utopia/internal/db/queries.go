package db

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/logger"
	"github.com/shehbaaz/db-utopia/internal/models"
	"github.com/shehbaaz/db-utopia/internal/parser"
)

type PanelConfig struct {
	IP    string
	Port  string
	MacID string
}

func BatchInsertPanelLogs(ctx context.Context, dbPool *pgxpool.Pool, panelLogData []models.PanelLog) error {

	if len(panelLogData) == 0 {
		return nil
	}

	// Prepare arrays
	panelIDs := make([]string, 0, len(panelLogData))
	clientIDs := make([]string, 0, len(panelLogData))
	ipAddresses := make([]string, 0, len(panelLogData))
	rvalues := make([]string, 0, len(panelLogData))
	lvalues := make([]string, 0, len(panelLogData))
	siaCodes := make([]string, 0, len(panelLogData))
	msgTypes := make([]string, 0, len(panelLogData))
	msgs := make([]string, 0, len(panelLogData))
	responses := make([]string, 0, len(panelLogData))
	msgFroms := make([]string, 0, len(panelLogData))
	msgTos := make([]string, 0, len(panelLogData))
	alertTimes := make([]time.Time, 0, len(panelLogData))
	receiveTimes := make([]time.Time, 0, len(panelLogData))
	isTicketCreated := make([]bool, 0, len(panelLogData))
	ticketRemarks := make([]string, 0, len(panelLogData))
	isAutoClosed := make([]bool, 0, len(panelLogData))
	sensorNames := make([]string, 0, len(panelLogData))

	for _, p := range panelLogData {
		panelIDs = append(panelIDs, p.PanelId)
		clientIDs = append(clientIDs, p.ClientId)
		ipAddresses = append(ipAddresses, p.IpAddress)
		rvalues = append(rvalues, p.Rvalue)
		lvalues = append(lvalues, p.Lvalue)
		siaCodes = append(siaCodes, p.SiaCode)
		msgTypes = append(msgTypes, p.MsgType)
		msgs = append(msgs, p.Msg)
		responses = append(responses, p.Response)
		msgFroms = append(msgFroms, p.MsgFrom)
		msgTos = append(msgTos, p.MsgTo)
		alertTimes = append(alertTimes, p.AlertTime)
		receiveTimes = append(receiveTimes, p.ReceiveTime)
		isTicketCreated = append(isTicketCreated, p.IsTicketCreated)
		ticketRemarks = append(ticketRemarks, p.TicketRemark)
		isAutoClosed = append(isAutoClosed, p.IsAutoClosed)
		if p.SensorName != "" {
			sensorNames = append(sensorNames, p.SensorName)
		} else {
			sensorNames = append(sensorNames, "Invalid Sensor")
		}
	}

	query := `
	INSERT INTO panel_log (
		panelid,
		client_id,
		ipaddress,
		rvalue,
		lvalue,
		siacode,
		message_type,
		message,
		response,
		msg_from,
		msg_to,
		alert_time,
		receive_time,
		is_ticket_created,
		"ticketRemark",
		is_auto_closed,
		sensor_name
	)
	SELECT *
	FROM UNNEST(
		$1::text[],
		$2::text[],
		$3::text[],
		$4::text[],
		$5::text[],
		$6::text[],
		$7::text[],
		$8::text[],
		$9::text[],
		$10::text[],
		$11::text[],
		$12::timestamptz[],
		$13::timestamptz[],
		$14::boolean[],
		$15::text[],
		$16::boolean[],
		$17::text[]
	)
	`

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err := dbPool.Exec(
		dbCtx,
		query,
		panelIDs,
		clientIDs,
		ipAddresses,
		rvalues,
		lvalues,
		siaCodes,
		msgTypes,
		msgs,
		responses,
		msgFroms,
		msgTos,
		alertTimes,
		receiveTimes,
		isTicketCreated,
		ticketRemarks,
		isAutoClosed,
		sensorNames,
	)

	if err != nil {
		logger.Errorf(
			"[PANEL-LOG-BATCH] failed | size=%d | err=%v | sample_panel=%s",
			len(panelLogData),
			err,
			panelIDs[0],
		)
		return err
	}

	if time.Since(start) > 5*time.Second {
		logger.Warnf(
			"[PANEL-LOG-BATCH] slow insert | duration=%v | size=%d",
			time.Since(start),
			len(panelLogData),
		)
	}

	return nil

}

func shortHash(panelID string) string {
	h := fnv.New32a()
	h.Write([]byte(panelID))
	return fmt.Sprintf("%03x", h.Sum32()%4096) // 3 hex chars
}

func updatePanelDVRIPs(
	ctx context.Context,
	tx pgx.Tx,
	panelID string,
	ip string,
) error {

	_, err := tx.Exec(ctx, `
		UPDATE panel_dvr pd
		SET dvr_ip = $1
		FROM panel_master pm
		JOIN panel_site ps
			ON ps.master_id = pm.id
		JOIN panel_site_dvr psd
			ON psd.site_id = ps.id
		WHERE pm.panelid = $2
		  AND pd.id = psd.dvr_id
	`, ip, panelID)

	return err
}

func updatePanelIPSmart(
	ctx context.Context,
	dbPool *pgxpool.Pool,
	panelID string,
	clientID string,
	ip string,
) error {

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := dbPool.Begin(dbCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(dbCtx)

	rows, err := tx.Query(dbCtx, `
		SELECT panelid
		FROM panel_master
		WHERE ipaddress = $1
		AND panelid != $2
		FOR UPDATE
	`, ip, panelID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var existingPanels []string

	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return err
		}
		existingPanels = append(existingPanels, pid)
	}

	// shift existing
	for _, pid := range existingPanels {

		// newIP := fmt.Sprintf("%s(change %d)", ip, i+1)
		newIP := fmt.Sprintf("%s#%s", ip, shortHash(panelID))
		_, err := tx.Exec(dbCtx, `
			UPDATE panel_master
			SET ipaddress = $1,
			    router_ip = $1,
			    dvr_ip = $1
			WHERE panelid = $2
		`, newIP, pid)

		if err != nil {
			return err
		}

		if err := updatePanelDVRIPs(dbCtx, tx, pid, newIP); err != nil {
			return err
		}
	}

	// update current panel
	_, err = tx.Exec(dbCtx, `
		UPDATE panel_master pm
		SET ipaddress = $1,
		    router_ip = $1,
		    dvr_ip = $1,
		    last_ip_update_on = NOW()
		FROM panel_site ps,
		     panel_business_unit pbu
		WHERE pm.panelid = $2
			AND ps.master_id = pm.id
			AND pbu.id = ps.bu_id
			AND pbu.client_id = $3
	`, ip, panelID, clientID)

	if err != nil {
		return err
	}

	if err := updatePanelDVRIPs(dbCtx, tx, panelID, ip); err != nil {
		return err
	}

	return tx.Commit(dbCtx)
}

func BatchUpdatePanelIP(ctx context.Context, dbPool *pgxpool.Pool, panelIPData []models.PanelIPUpdate) error {

	if len(panelIPData) == 0 {
		return nil
	}

	panelIDs := make([]string, 0, len(panelIPData))
	clientIDs := make([]string, 0, len(panelIPData))
	panelIPs := make([]string, 0, len(panelIPData))

	for _, p := range panelIPData {
		panelIDs = append(panelIDs, p.PanelId)
		clientIDs = append(clientIDs, p.ClientId)
		panelIPs = append(panelIPs, p.PanelIP)
	}

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := dbPool.Begin(dbCtx)
	if err != nil {
		return err
	}
	defer tx.Rollback(dbCtx)

	query := `
		UPDATE panel_master pm
		SET ipaddress = data.panel_ip,
			router_ip = data.panel_ip,
			dvr_ip = data.panel_ip,
			last_ip_update_on = NOW()
		FROM UNNEST(
			$1::varchar[], 
			$2::varchar[], 
			$3::varchar[]
		)
		AS data(panelid, client_id, panel_ip),
		panel_site ps,
		panel_business_unit pbu
		WHERE pm.panelid = data.panelid
			AND ps.master_id = pm.id
			AND pbu.id = ps.bu_id
			AND pbu.client_id = data.client_id
			AND pm.ipaddress IS DISTINCT FROM data.panel_ip
	`

	_, err = tx.Exec(dbCtx, query, panelIDs, clientIDs, panelIPs)
	if err != nil {

		pgErr, ok := err.(*pgconn.PgError)

		if !ok || pgErr.Code != "23505" {
			return err
		}

		logger.Warnf(
			"[UPDATE-IP-BATCH] conflict detected, fallback to smart update | size=%d",
			len(panelIPData),
		)

		for _, p := range panelIPData {

			err := updatePanelIPSmart(
				context.Background(),
				dbPool,
				p.PanelId,
				p.ClientId,
				p.PanelIP,
			)

			if err != nil {
				logger.Errorf(
					"[UPDATE-IP-FALLBACK] panel=%s err=%v",
					p.PanelId,
					err,
				)
			}
		}

		return nil
	}

	dvrUpdateQuery := `
		UPDATE panel_dvr pd
		SET dvr_ip = data.panel_ip
		FROM (
			SELECT DISTINCT
				psd.dvr_id,
				u.panel_ip
			FROM UNNEST(
				$1::varchar[],
				$2::varchar[],
				$3::varchar[]
			) AS u(panelid, client_id, panel_ip)
			JOIN panel_master pm
				ON pm.panelid = u.panelid
			JOIN panel_site ps
				ON ps.master_id = pm.id
			JOIN panel_site_dvr psd
				ON psd.site_id = ps.id
			JOIN panel_business_unit pbu
				ON pbu.id = ps.bu_id
			   AND pbu.client_id = u.client_id
		) data
		WHERE pd.id = data.dvr_id
	`
	// panel_dvr update
	_, err = tx.Exec(dbCtx, dvrUpdateQuery, panelIDs, clientIDs, panelIPs)

	if err != nil {
		return err
	}

	if err := tx.Commit(dbCtx); err != nil {
		return err
	}

	logger.Infof(
		"[UPDATE-IP-BATCH] success | size=%d",
		len(panelIPData),
	)

	return nil
}

func BatchUpsertPanelHeartbeat(
	ctx context.Context,
	dbPool *pgxpool.Pool,
	data []models.PanelHeartBeat,
) error {

	if len(data) == 0 {
		return nil
	}

	// Deduplicate by panelid + client_id
	unique := make(map[string]models.PanelHeartBeat)

	for _, d := range data {

		key := d.PanelID + "|" + d.ClientID

		if existing, ok := unique[key]; ok {

			// keep latest heartbeat
			if d.ReceiveTime.After(existing.ReceiveTime) {
				unique[key] = d
			}

			continue
		}

		unique[key] = d
	}

	panelIDs := make([]string, 0, len(unique))
	clientIDs := make([]string, 0, len(unique))
	panelIPs := make([]string, 0, len(unique))
	statuses := make([]bool, 0, len(unique))
	datetimes := make([]time.Time, 0, len(unique))
	sources := make([]string, 0, len(unique))

	for _, d := range unique {
		panelIDs = append(panelIDs, d.PanelID)
		clientIDs = append(clientIDs, d.ClientID)
		panelIPs = append(panelIPs, d.PanelIP)
		statuses = append(statuses, true)
		datetimes = append(datetimes, d.ReceiveTime)
		sources = append(sources, d.Source)
	}

	query := `
	INSERT INTO reports_all_panels (
		panelid,
		client_id,
		panel_ip,
		status,
		datetime,
		source,
		last_heartbeat
	)
	SELECT 
		panelid,
		client_id,
		panel_ip,
		status,
		datetime,
		source,
		last_heartbeat
// convert multiple arrays into rows and assign column names to them.
FROM UNNEST(
		$1::text[],
		$2::text[],
		$3::text[],
		$4::boolean[],
		$5::timestamptz[],
		$6::text[],
		$7::timestamptz[]
	) AS t(panelid, client_id, panel_ip, status, datetime, source, last_heartbeat)
	ON CONFLICT (panelid, client_id)
	DO UPDATE
	SET
	    panel_ip = EXCLUDED.panel_ip,
	    status = EXCLUDED.status,
	    datetime = EXCLUDED.datetime,
	    source = EXCLUDED.source,
		last_heartbeat = EXCLUDED.last_heartbeat
	`

	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	count, err := dbPool.Exec(
		dbCtx,
		query,
		panelIDs,
		clientIDs,
		panelIPs,
		statuses,
		datetimes,
		sources,
		datetimes,
	)
	if err != nil {
		logger.Warnf(
			"[HEARTBEAT-BATCH] failed | size=%d | err=%v",
			len(data),
			err,
		)
		return err
	}

	logger.Infof(
		"[HEARTBEAT-BATCH] attempted upsert | affected rows=%d | size=%d",
		count.RowsAffected(),
		len(data),
	)

	return nil
}

func GetPanelConfigByPanelID(dbPool *pgxpool.Pool, panelID string) (models.PanelData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var panel models.PanelData
	query := `
		SELECT 
			pm.id,
			pm.panelid,
			COALESCE(pm.mac_id, '') AS mac_id,
			pbu.client_id
		FROM panel_master pm
		JOIN panel_site ps ON ps.master_id = pm.id
		JOIN panel_business_unit pbu ON pbu.id = ps.bu_id
		WHERE pm.panelid = $1
		LIMIT 1
	`
	err := dbPool.QueryRow(ctx, query, panelID).
		Scan(&panel.PanelPK, &panel.PanelID, &panel.MacID, &panel.ClientID)

	if err != nil {
		return panel, err
	}

	return panel, nil

}
func GetPanelConfigByPanelIDClientID(dbPool *pgxpool.Pool, panelID, clientID string) (models.PanelData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var panel models.PanelData
	query := `
		SELECT 
			pm.id,
			pm.panelid,
			COALESCE(pm.mac_id, '') AS mac_id,
			pbu.client_id
		FROM panel_master pm
		JOIN panel_site ps ON ps.master_id = pm.id
		JOIN panel_business_unit pbu ON pbu.id = ps.bu_id
		WHERE pm.panelid = $1 AND pbu.client_id = $2
		LIMIT 1
	`
	err := dbPool.QueryRow(ctx, query, panelID, clientID).
		Scan(&panel.PanelPK, &panel.PanelID, &panel.MacID, &panel.ClientID)

	if err != nil {
		return panel, err
	}

	return panel, nil

}

func GetPanelConfigByMacID(dbPool *pgxpool.Pool, macID string) (models.PanelData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var panel models.PanelData
	query := `
		SELECT 
			pm.id,
			pm.panelid,
			pm.mac_id,
			pbu.client_id
		FROM panel_master pm
		JOIN panel_site ps ON ps.master_id = pm.id
		JOIN panel_business_unit pbu ON pbu.id = ps.bu_id
		WHERE pm.mac_id = $1
		LIMIT 1
	`
	err := dbPool.QueryRow(ctx, query, macID).
		Scan(&panel.PanelPK, &panel.PanelID, &panel.MacID, &panel.ClientID)

	if err != nil {
		return panel, err
	}

	return panel, nil
}

func SetPanelIpAndVersion(dbPool *pgxpool.Pool, panelPK int64, panelIP, panelVersion string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tx, err := dbPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE 
			panel_master 
		SET 
			ipaddress =$1,
			router_ip =$1,
			dvr_ip  = $1,
			version = $2
		WHERE 
			id=$3
	`
	_, err = tx.Exec(ctx, query, panelIP, panelVersion, panelPK)

	if err != nil {
		return err
	}

	updateDvrQuery := `
		UPDATE panel_dvr pd
		SET dvr_ip = $1
		FROM panel_site ps
		JOIN panel_site_dvr psd
			ON psd.site_id = ps.id
		WHERE ps.master_id = $2
		  AND pd.id = psd.dvr_id
	`

	_, err = tx.Exec(ctx, updateDvrQuery, panelIP, panelPK)

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func UpdatePanelArmDisArmStatus(dbPool *pgxpool.Pool, panelPK string, isArm bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	query := `
		UPDATE 
			panel_master 
		SET 
			armed =$2,
			arm_time=NOW()
		WHERE 
			id=$1
	`
	_, err := dbPool.Exec(
		ctx,
		query,
		panelPK,
		isArm,
	)

	return err
}

func UpsertPanelSensorStatus(dbPool *pgxpool.Pool, panelPK string, sensorData []parser.ZoneStatus) error {
	if len(sensorData) == 0 {
		return nil
	}
	zones := make([]string, 0, len(sensorData))
	statuses := make([]string, 0, len(sensorData))
	sensorTypes := make([]string, 0, len(sensorData))

	for _, s := range sensorData {
		zones = append(zones, s.Zone)
		statuses = append(statuses, s.Status)
		sensorTypes = append(sensorTypes, s.SensorName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := `
		INSERT INTO panel_panelzonestatus (
			zone,
			sensor_type,
			status,
			last_updated,
			panel_id
		)
		SELECT 
			zone,
			sensor_type,
			status,
			NOW(),
			$1
		FROM UNNEST(
			$2::text[],
			$3::text[],
			$4::text[]
		) AS t(zone, status, sensor_type)
		ON CONFLICT (panel_id, zone)
		DO UPDATE
		SET
			status = EXCLUDED.status,
			sensor_type = EXCLUDED.sensor_type,
			last_updated = NOW()
		WHERE
			panel_panelzonestatus.status IS DISTINCT FROM EXCLUDED.status
			OR panel_panelzonestatus.sensor_type IS DISTINCT FROM EXCLUDED.sensor_type
	`
	_, err := dbPool.Exec(
		ctx,
		query,
		panelPK,
		zones,
		statuses,
		sensorTypes,
	)
	if err != nil {
		logger.Errorf("[ZONE-UPSERT] failed | panel=%s size=%d err=%v",
			panelPK, len(sensorData), err)
		return err
	}

	return nil
}

func InsertPanelSensorStatusLog(dbPool *pgxpool.Pool, panelID string, sensorData []parser.ZoneStatus) error {
	if len(sensorData) == 0 {
		return nil
	}
	zones := make([]string, 0, len(sensorData))
	statuses := make([]string, 0, len(sensorData))
	sensorTypes := make([]string, 0, len(sensorData))
	alertTimes := make([]time.Time, 0, len(sensorData))

	for _, s := range sensorData {
		zones = append(zones, s.Zone)
		statuses = append(statuses, s.Status)
		sensorTypes = append(sensorTypes, s.SensorName)
		alertTimes = append(alertTimes, s.AlertTime)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	query := `
		INSERT INTO panel_panelzonestatuslog (
			zone,
			sensor_type,
			status,
			created_at,
			panel_id,
			alert_time
		)
		SELECT 
			zone,
			sensor_type,
			status,
			NOW(),
			$1,
			t.alert_time
		FROM UNNEST(
			$2::text[],
			$3::text[],
			$4::text[],
			$5::timestamp[]
		) AS t(zone, status, sensor_type, alert_time)
	`
	count, err := dbPool.Exec(
		ctx,
		query,
		panelID,
		zones,
		statuses,
		sensorTypes,
		alertTimes,
	)

	if err != nil {
		logger.Errorf("[ZONE-LOG] failed | panel=%s size=%d err=%v",
			panelID, len(sensorData), err)
		return err
	}
	logger.Infof("[ZONE-LOG] inserted rows=%d | panel=%s size=%d", count.RowsAffected(), panelID, len(sensorData))
	return nil
}

func GetPanelWithSensorFromDB(ctx context.Context, db *pgxpool.Pool, panelPK string) (map[string]string, error) {

	query := `
	SELECT pzp."alarm_code",pzp."restore_code", pzp."SensorType", pzp.zone
	FROM panel_master p
	JOIN panel_site s ON p.id = s.master_id
	JOIN panel_zone_profile pzp
	    ON pzp.profile_id = p.profile_id
	    AND pzp.bu_id = s.bu_id
	    AND pzp.site_id = s.id
	WHERE p.id = $1
	`

	rows, err := db.Query(ctx, query, panelPK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)

	for rows.Next() {
		var alarmCode, restoreCode, sensorType, zone string
		if err := rows.Scan(&alarmCode, &restoreCode, &sensorType, &zone); err != nil {
			return nil, err
		}
		result[alarmCode] = sensorType
		result[zone] = sensorType
		if restoreCode != "" {
			result[restoreCode] = sensorType
		}
	}

	return result, nil
}

func LoadZoneConfig(
	ctx context.Context,
	db *pgxpool.Pool,
	panelID,
	clientID string,
) (
	map[string]models.ZoneConfig,
	error,
) {
	query := `
		select
			pzp.zone,
			pzp.alarm_code,
			pzp.restore_code
		from panel_zone_profile pzp
		join panel_site ps
			on ps.id = pzp.site_id
		join panel_business_unit pbu
			on pbu.id = ps.bu_id
		join panel_master pm
			on pm.id = ps.master_id
		where
			pm.panelid = $1
			and pbu.client_id = $2
			and pzp.is_restore != true
		`

	rows, err := db.Query(
		ctx,
		query,
		panelID,
		clientID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	cfg := make(
		map[string]models.ZoneConfig,
	)

	for rows.Next() {

		var (
			zoneNo  string
			alarm   string
			restore string
		)

		if err := rows.Scan(
			&zoneNo,
			&alarm,
			&restore,
		); err != nil {

			return nil, err
		}

		cfg[zoneNo] = models.ZoneConfig{
			Alarm:   alarm,
			Restore: restore,
		}
	}

	return cfg, nil
}

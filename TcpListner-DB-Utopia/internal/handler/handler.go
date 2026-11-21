package handler

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"

	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/constants"
	"github.com/shehbaaz/db-utopia/internal/db"
	"github.com/shehbaaz/db-utopia/internal/logger"
	"github.com/shehbaaz/db-utopia/internal/models"
	"github.com/shehbaaz/db-utopia/internal/parser"
	"github.com/shehbaaz/db-utopia/internal/redis"
	"github.com/shehbaaz/db-utopia/internal/utils"
	"golang.org/x/net/http2"
)

type EnqueueLogFunc func(models.PanelLog)

type EnqueueHeartBeatFunc func(models.PanelHeartBeat)

type PanelConfig struct {
	MacID      string
	ClientID   string
	PanelID    string
	ReceiverID string
	IP         string
	Port       string
	Status     string
}

var sensorStatusMap = map[string]string{
	"A": "ALARM",
	"R": "RESTORE",
	"U": "UNINSTALLED",
	"B": "BYPASS",
	"1": "ALARM",
	"0": "RESTORE",
	"2": "UNINSTALLED",
}

var otherCommandTypeMap = map[string]string{
	"NULL":         "Periodic Report",
	"NOA":          "DISARM",
	"NCL":          "ARM",
	"NBA562":       "EML OPEN FROM KEYPAD",
	"NBA558":       "PASSWORD LOGGED IN FROM KEYPAD",
	"NWC":          "WRONG PASSWORD ENTRY",
	"NBA559":       "UTILITY LOGIN",
	"NBA561":       "HOOTER ACK FROM KEYPAD",
	"NHA000":       "HOOTER ACK FROM PANEL HOOTER ACK SWITCH",
	"NBA563":       "PARTITION ARM-1 EVENT",
	"NBA564":       "PARTITION ARM-1 EVENT",
	"NED400":       "DISARM EXTENDED BY ADMIN",
	"NPA000":       "PANIC ALERT",
	"NFA501":       "HOOTER-1 FAIL",
	"NFA502":       "HOOTER -2 FAIL",
	"NYY651][N|T1": "TEMPERATURE VALUE",
	"NYY652][N|L":  "LIGHT LUX VALUE",
	"NPA565":       "PIR WATCH DOG ALERT ",
	"NBA566":       "KEYPAD CONFIGURATION UPDATE",
	"NCC":          "USER PASSWORD CHANGED ALERT",
	"NBA555":       "BATTERY LOW ALERT",
	"NEA701":       "EXPANDER CARD (A TYPE) DISCONNECT EVENT",
	"NER701":       "EXPANDER CARD (A TYPE) CONNECT EVENT",
	"NEA702":       "EXPANDER CARD (B TYPE) DISCONNECT EVENT",
	"NER702":       "EXPANDER CARD (B TYPE) CONNECT EVENT",
	"NEA703":       "EXPANDER CARD (C TYPE) DISCONNECT EVENT",
	"NER703":       "EXPANDER CARD (C TYPE) CONNECT EVENT",
	"NBA553":       "TRANSACTION PENDING COUNT CLEAR",
	"NBA556":       "ALARM LOG COUNT CLEAR",
	"NBA557":       "EVENT LOG COUNT CLEAR",
	"NSA400":       "SILENT_ARMING (ADMIN)",
	"NFA400":       "FORCE_FULL_ARMING (ADMIN)",
	"NNA400":       "NIGHT_ARMING (ADMIN)  ",
	"NDA400":       "ADMIN DISARM FROM KEYPAD",
	"NBA560":       "ENGINEER LOGIN FROM KEYPAD",
}

func GetSiaMessageType(SIACode string) string {

	code := strings.TrimSpace(SIACode)

	// 1️⃣ Exact match first (fast path)
	if msgType, exists := otherCommandTypeMap[code]; exists {
		return msgType
	}

	// 2️⃣ Try contains match
	for k, v := range otherCommandTypeMap {
		if strings.Contains(code, k) {
			return v
		}
	}

	// 3️⃣ Fallback logic (existing)
	base := strings.Split(code, "|")[0]

	if len(base) >= 6 {
		base = base[:3]
		switch {
		case strings.HasSuffix(base, "A"):
			return "ALARM"
		case strings.HasSuffix(base, "R"):
			return "ALARM RESTORAL"
		}
	}

	return "UNKNOWN"
}

func HandleOtherCommands(data *parser.SIAData, SiaCode string, panelLogData *models.PanelLog,
	dbPool *pgxpool.Pool, logFunc EnqueueLogFunc, panelSensorCache *redis.PanelSensorCache) {
	siaMessage := otherCommandTypeMap[SiaCode]
	switch {
	case strings.Contains(SiaCode, constants.ArmCommand): //NCL
		panelLogData.SensorName = "ArmCommand"
		handleArmCommand(SiaCode, data, dbPool, panelLogData, logFunc)
	case strings.Contains(SiaCode, constants.DisarmCommand): //NOA
		panelLogData.SensorName = "DisArmCommand"
		handleDisArmCommand(SiaCode, data, dbPool, panelLogData, logFunc)
	default:
		logger.Infof("Handling Other Command: %s for PanelID: %s", siaMessage, data.PanelID)
		SetSensorStatus(dbPool, data, panelSensorCache)
		panelLogData.MsgType = siaMessage
		panelLogData.SensorName = siaMessage
		logFunc(*panelLogData)
	}

}

func SetSensorStatus(dbPool *pgxpool.Pool, siaData *parser.SIAData, panelSensorCache *redis.PanelSensorCache) {
	sensorData, ok := siaData.ExtractZoneStatuses()
	if !ok || len(sensorData) == 0 {
		logger.Warnf("[SENSOR] no data | panel=%s", siaData.PanelID)
		return
	}
	// 🔹 Get panel sensor map
	panelMap, err := panelSensorCache.Get(siaData.PanelPK)
	if err != nil {
		logger.Warnf("[SENSOR] cache fetch failed panel=%s err=%v", siaData.PanelID, err)
	}

	alerttime, err := siaData.Timestamp()
	if err != nil {
		logger.Errorf("Errorf parsing timestamp: %v", err)
		alerttime = time.Now().In(utils.IST)

	}

	// Enrich sensor names
	for i := range sensorData {

		sensorName := ""

		// best case → SiaCode exists
		if panelMap != nil && sensorData[i].Zone != "" {
			if val, ok := panelMap[sensorData[i].Zone]; ok {
				sensorName = val
				break
			}
		}

		sensorData[i].SensorName = sensorName
		sensorData[i].AlertTime = alerttime
	}

	// 1 UPSERT latest state
	err = db.UpsertPanelSensorStatus(dbPool, siaData.PanelPK, sensorData)
	if err != nil {
		logger.Errorf(
			"[SENSOR] upsert failed | panel=%s err=%v",
			siaData.PanelID,
			err,
		)
	}
	// 2 INSERT log (history)
	err = db.InsertPanelSensorStatusLog(dbPool, siaData.PanelPK, sensorData)
	if err != nil {
		logger.Errorf(
			"[SENSOR] log insert failed | panel=%s err=%v",
			siaData.PanelID,
			err,
		)
	}
}

func HandleHeartBeat(data *parser.NULLData, panelLogData *models.PanelLog, logFunc EnqueueLogFunc, heartBeatFunc EnqueueHeartBeatFunc) {

	panelHeartBeat := models.PanelHeartBeat{
		PanelID:     data.PanelID,
		ClientID:    data.ClientID,
		PanelIP:     data.PanelIP,
		Source:      string(constants.HeartBeat),
		ReceiveTime: time.Now(),
	}

	heartBeatFunc(panelHeartBeat)
	logFunc(*panelLogData)

}
func handleArmCommand(SiaCode string, data *parser.SIAData, dbPool *pgxpool.Pool, panelLogData *models.PanelLog, logFunc EnqueueLogFunc) {
	panelLogData.MsgType = GetSiaMessageType(SiaCode)

	err := db.UpdatePanelArmDisArmStatus(dbPool, data.PanelPK, true)
	if err != nil {
		logger.Errorf("Unable to Update Panel Arm in DB for Panel %s and Err %s", data.PanelID, err)
	}

	logFunc(*panelLogData)
}

func handleDisArmCommand(SiaCode string, data *parser.SIAData, dbPool *pgxpool.Pool, panelLogData *models.PanelLog, logFunc EnqueueLogFunc) {
	panelLogData.MsgType = GetSiaMessageType(SiaCode)
	err := db.UpdatePanelArmDisArmStatus(dbPool, data.PanelPK, false)
	if err != nil {
		logger.Errorf("Unable to Update Panel Dis Arm in DB for Panel %s", data.PanelID)
	}
	logFunc(*panelLogData)
}

func ParseGenericSIA(siaCode string) (zone string, status string, err error) {
	code := strings.TrimSpace(siaCode)
	if len(code) < 6 {
		return "", "", fmt.Errorf("invalid SIA code: %s", code)
	}
	// status is 3rd char (index 2)
	statusCode := string(code[2])

	status, ok := sensorStatusMap[statusCode]
	if !ok {
		return "", "", fmt.Errorf("unknown status code: %s", statusCode)
	}

	// zone is last 3 chars
	zoneStr := code[len(code)-3:]

	return zoneStr, status, nil
}

func UpdateSensorStatus(
	dbPool *pgxpool.Pool,
	siaData *parser.SIAData,
	panelSensorCache *redis.PanelSensorCache,
) error {

	siaCode := strings.TrimSpace(siaData.SiaCode)

	if len(siaCode) == 0 {
		return fmt.Errorf("empty SIA code")
	}
	// 🔹 Case 2: Generic sensor update (NBA001, NBR002, etc)
	siaCode = strings.TrimSpace(siaCode)
	if len(siaCode) >= 32 {
		err := UpdateMultipleSensorStatus(dbPool, siaData, panelSensorCache)
		if err != nil {
			return err
		}
		return nil
	}
	zone, status, err := ParseGenericSIA(siaCode)
	if err != nil {
		return err
	}

	panelMap, err := panelSensorCache.Get(siaData.PanelPK)
	if err != nil {
		logger.Warnf("[SENSOR] cache fetch failed panel=%s err=%v", siaData.PanelID, err)
	}

	sensorName := "Unassigned Sensor"
	if panelMap != nil {
		if val, ok := panelMap[siaCode]; ok {
			sensorName = val
		}
	}

	logger.Infof(
		"[SENSOR] update | panel=%s zone=%s status=%s sensor=%s",
		siaData.PanelID,
		zone,
		status,
		sensorName,
	)

	alerttime, err := siaData.Timestamp()
	if err != nil {
		logger.Errorf("Errorf parsing timestamp: %v", err)
		alerttime = time.Now().In(utils.IST)

	}
	sensorData := []parser.ZoneStatus{
		{
			Zone:       zone,
			Status:     status,
			SensorName: sensorName,
			AlertTime:  alerttime,
		},
	}

	// 🔹 DB updates
	if err := db.UpsertPanelSensorStatus(dbPool, siaData.PanelPK, sensorData); err != nil {
		return err
	}

	if err := db.InsertPanelSensorStatusLog(dbPool, siaData.PanelPK, sensorData); err != nil {
		logger.Errorf("[SENSOR] log failed | err=%v", err)
	}

	return nil
}

func UpdateMultipleSensorStatus(dbPool *pgxpool.Pool, siaData *parser.SIAData, panelSensorCache *redis.PanelSensorCache) error {
	var sensorData []parser.ZoneStatus
	panelMap, err := panelSensorCache.Get(siaData.PanelPK)
	if err != nil {
		logger.Warnf("[SENSOR] cache fetch failed panel=%s err=%v", siaData.PanelID, err)
	}
	alerttime, err := siaData.Timestamp()
	if err != nil {
		logger.Errorf("Errorf parsing timestamp: %v", err)
		alerttime = time.Now().In(utils.IST)

	}
	for i, char := range siaData.SiaCode {
		status, ok := sensorStatusMap[string(char)]
		if !ok {
			logger.Warnf("[SENSOR] unknown status code: %s", string(char))
			continue
		}

		sensorName := "Unassigned Sensor"
		zoneStr := fmt.Sprintf("%03d", i+1)
		if panelMap != nil {
			if val, ok := panelMap[zoneStr]; ok {
				sensorName = val
			}
		}

		logger.Infof(
			"[SENSOR] update | panel=%s zone=%s status=%s sensor=%s",
			siaData.PanelID,
			zoneStr,
			status,
			sensorName,
		)
		sensorData = append(sensorData, parser.ZoneStatus{
			Zone:       zoneStr,
			Status:     status,
			SensorName: sensorName,
			AlertTime:  alerttime,
		})
	}

	// 🔹 DB updates
	if err := db.UpsertPanelSensorStatus(dbPool, siaData.PanelPK, sensorData); err != nil {
		logger.Errorf(
			"[SENSOR] upsert failed | panel=%s err=%v",
			siaData.PanelID,
			err,
		)
		return err
	}

	// if err := db.InsertPanelSensorStatusLog(dbPool, siaData.PanelPK, sensorData); err != nil {
	// 	logger.Errorf("[SENSOR] log failed | err=%v", err)
	// 	return err
	// }
	return nil
}

// Ticket creation with http.Client (plain)
func CreateTicket(data *parser.SIAData, SIACode, alerttime, djangoHost string) (*models.TicketAPIResponse, error) {
	url := fmt.Sprintf("%s/ticketcreate_safebox", djangoHost)

	payload := models.TicketAPIRequest{
		PanelID:   data.PanelID,
		SIACode:   SIACode,
		AlertTime: alerttime,
		ClientID:  data.ClientID,
	}
	jsonData, _ := json.Marshal(payload)

	logger.Infof("Sending payload: %+v", string(jsonData))

	var client *http.Client
	if strings.HasPrefix(url, "https://") {
		// HTTP/2 for production
		tr := &http2.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // ⚠️ Only for dev/self-signed certs
			},
		}

		client = &http.Client{Transport: tr, Timeout: 5 * time.Second}
	} else {
		// HTTP/1.1 for local testing
		client = &http.Client{
			Timeout: 5 * time.Second,
		}
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("Error calling ticket create: %s", err)
		return nil, fmt.Errorf("failed to ticket create with payload %s", jsonData)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result models.TicketAPIResponse

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %s", string(body))
	}

	if resp.StatusCode != 200 {
		return &result, fmt.Errorf("api returned error %s", result.Error)
	}

	return &result, nil
}

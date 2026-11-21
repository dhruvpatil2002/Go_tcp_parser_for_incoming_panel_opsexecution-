package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shehbaaz/db-utopia/internal/config"
	"github.com/shehbaaz/db-utopia/internal/constants"
	"github.com/shehbaaz/db-utopia/internal/db"
	"github.com/shehbaaz/db-utopia/internal/handler"
	"github.com/shehbaaz/db-utopia/internal/logger"
	"github.com/shehbaaz/db-utopia/internal/models"
	"github.com/shehbaaz/db-utopia/internal/parser"
	"github.com/shehbaaz/db-utopia/internal/redis"
	"github.com/shehbaaz/db-utopia/internal/utils"
)

type Server struct {
	cfg *config.Config
	// ports               []int
	port                string
	wg                  sync.WaitGroup
	pool                *pgxpool.Pool
	listeners           []net.Listener
	zoneAlertCache      *redis.ZoneAlertCache
	panelZoneDataCache  *redis.PanelZoneDataCache
	panelZoneStateCache *redis.PanelZoneStateCache
	panelSensorCache    *redis.PanelSensorCache
	panelDataCache      *redis.PanelDataCache
	logQueue            chan models.PanelLog
	updatePanelQueue    chan models.PanelIPUpdate
	panelHeartBeatQueue chan models.PanelHeartBeat
}

// func NewServer(cfg *config.Config, ports []int, pool *pgxpool.Pool,
func NewServer(cfg *config.Config, 
	port string, 
	pool *pgxpool.Pool,
	zoneAlertCache *redis.ZoneAlertCache, 
	panelZoneDataCache *redis.PanelZoneDataCache, 
	panelZoneStateCache *redis.PanelZoneStateCache,
	panelSensorCache *redis.PanelSensorCache, 
	panelDataCache *redis.PanelDataCache,
	logQueue chan models.PanelLog, 
	updatePanelQueue chan models.PanelIPUpdate,
	panelHeartBeatQueue chan models.PanelHeartBeat) *Server {
	return &Server{
		cfg: cfg,
		port: port,
		pool: pool,
		zoneAlertCache: zoneAlertCache,
		panelZoneDataCache: panelZoneDataCache, 
		panelZoneStateCache: panelZoneStateCache,
		panelSensorCache: panelSensorCache, 
		panelDataCache: panelDataCache,
		logQueue: logQueue, 
		updatePanelQueue: updatePanelQueue, 
		panelHeartBeatQueue: panelHeartBeatQueue,}
}

var otherCommandsMap = map[string]struct{}{
	"NBA562":       {},
	"NBA558":       {},
	"NWC":          {},
	"NBA559":       {},
	"NBA561":       {},
	"NHA000":       {},
	"NBA563":       {},
	"NBA564":       {},
	"NED400":       {},
	"NPA000":       {},
	"NFA501":       {},
	"NFA502":       {},
	"NYY651][N|T1": {},
	"NYY652][N|L":  {},
	"NYY656][N":    {},
	"NYY655][N":    {},
	"NPA565":       {},
	"NBA566":       {},
	"NCC":          {},
	"NBA555":       {},
	"NEA701":       {},
	"NER701":       {},
	"NEA702":       {},
	"NER702":       {},
	"NEA703":       {},
	"NER703":       {},
	"NBA553":       {},
	"NBA556":       {},
	"NBA557":       {},
	"NSA400":       {},
	"NFA400":       {},
	"NNA400":       {},
	"NDA400":       {},
	"NCL400":       {},
	"NBA560":       {},
	"NOA400":       {},
}

func isExistInOtherCommands(SIACode string) bool {
	_, exists := otherCommandsMap[strings.TrimSpace(SIACode)]
	return exists
}

func (s *Server) Start(ctx context.Context) {
	// for _, p := range s.ports {
	// 	s.wg.Add(1)
	// 	go s.listen(ctx, p)
	// }
	s.wg.Add(1)
	port, err := strconv.Atoi(s.port)
	if err != nil {
		logger.Errorf("Invalid TCP port: %s", s.port)
		return
	}
	go s.listen(ctx, port)
	<-ctx.Done()
}

func (s *Server) listen(ctx context.Context, port int) {
	defer s.wg.Done()
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.cfg.TcpHost, port))
	if err != nil {
		logger.Infof("Port %d bind error: %v", port, err)
		return
	}
	s.listeners = append(s.listeners, ln)
	logger.Infof("(MAIN) Listening on %s:%d", s.cfg.TcpHost, port)

	for {
		select {
			//The loop continuously checks whether shutdown has been requested.
			//If not, it attempts to accept another connection.
		case <-ctx.Done():
			_ = ln.Close()
			return
		default:
			err := ln.(*net.TCPListener).SetDeadline(time.Now().Add(15 * time.Second))
			if err != nil {
				logger.Errorf("Error in setting Deadline to connection")
			}
			conn, err := ln.Accept()
			if err != nil {
				if opErr, ok := err.(net.Error); ok && opErr.Timeout() {
					continue
				}
				logger.Errorf("Accept error on port %d: %v", port, err)
				continue
			}
			go s.handle(conn)
		}
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	panelIP, reader, err := utils.ReadPanelIP(conn)
	if strings.Contains(panelIP, "10.80.8.246") || strings.Contains(panelIP, "10.80.8.245") {
		return
	}
	if err != nil {
		logger.Errorf("Failed to read panel IP: %v", err)
		return
	}
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		cmd := string(data)
		logger.Infof("Received Alert from IP=%s  | Data=%q | Time=%s", panelIP, cmd, time.Now().Format(time.RFC3339))

		if nd, ok := parser.ParseNULL(cmd); ok {
			nd.PanelIP = panelIP
			logger.Infof("NULL message parsed (Securico): %+v", nd)
			// nak := nd.GetNAK()
			now := time.Now()
			alerttime := nd.Timestamp
			duration := time.Since(alerttime)
			ack := nd.GetACK()
			if duration.Seconds() > 180 || duration.Seconds() < -180 {
				logger.Warnf("Alert time %s differs from server time %s by more than 3 minutes", alerttime.Format(time.RFC3339), now.Format(time.RFC3339))
				// response = d.GetNAK()
				ack = nd.GetNAK()
				logger.Infof("Sent NAK due to time discrepancy : %q", ack)
			}
			_, err := conn.Write([]byte(ack))
			if err != nil {
				logger.Errorf("Error in sending ACK %s to panel id %s", ack, nd.PanelID)
			}
			logger.Infof("Sent ACK (Securico): %q | Closing connection", ack)
			panelLogData := models.PanelLog{
				PanelId:     nd.PanelID,
				ClientId:    nd.ClientID,
				IpAddress:   panelIP,
				Rvalue:      nd.Receiver,
				Lvalue:      nd.ClientID,
				SiaCode:     "NULL",
				MsgType:     "Periodic Report",
				Msg:         cmd,
				Response:    ack,
				MsgFrom:     constants.PANEL,
				MsgTo:       constants.APPLICATION,
				AlertTime:   nd.Timestamp,
				ReceiveTime: time.Now().In(utils.IST),
				SensorName:  string(constants.HeartBeat),
			}
			go handler.HandleHeartBeat(nd, &panelLogData, s.enqueueLog, s.enqueueHeartBeat)

			return // Close after ACK
		}

		// parse SIA
		if d, ok := parser.ParseSIA(cmd); ok {
			d.PanelIP = panelIP
			logger.Infof("SIA Message Parsed (Securico): %s", d)

			alerttime, err := d.Timestamp()
			if err != nil {
				logger.Errorf("Errorf parsing timestamp: %v", err)
				return
			}

			logger.Infof(
				"SIA Command=%s PanelID=%s ClientID=%s Data=%s",
				d.SiaCode, d.PanelID, d.ClientID, data,
			)

			updatePanelIp := models.PanelIPUpdate{
				PanelId:  d.PanelID,
				ClientId: d.ClientID,
				PanelIP:  panelIP,
			}

			go s.enqueueUpdatePanel(updatePanelIp)

			panelLogData := models.PanelLog{
				PanelId:     d.PanelID,
				ClientId:    d.ClientID,
				IpAddress:   panelIP,
				Rvalue:      d.Receiver,
				Lvalue:      d.ClientID,
				SiaCode:     d.SiaCode,
				MsgType:     handler.GetSiaMessageType(d.SiaCode),
				Msg:         cmd,
				MsgFrom:     constants.PANEL,
				MsgTo:       constants.APPLICATION,
				AlertTime:   alerttime,
				ReceiveTime: time.Now().In(utils.IST),
			}

			response := d.GetACK()

			_, err = conn.Write([]byte(response))
			if err != nil {
				panelLogData.Response = err.Error()
				logger.Errorf("Error in sending response %s to panel id %s", response, d.PanelID)
			} else {
				panelLogData.Response = response
			}

			logger.Infof("Response Sent : %q", response)

			var panelData *models.PanelData

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			panelData, err = s.panelDataCache.Get(
				ctx,
				d.PanelID,
				d.ClientID,
				func(ctx context.Context, panelID, clientID string) (*models.PanelData, error) {

					d, err := db.GetPanelConfigByPanelIDClientID(s.pool, panelID, clientID)
					if err != nil {
						return nil, err
					}
					return &d, nil
				},
			)
			cancel()
			if err != nil {
				logger.Warnf("MAC fetch failed panel=%s err=%v", d.PanelID, err)
			}
			if panelData != nil {
				d.PanelPK = panelData.PanelPK
				panelLogData.PanelPK = panelData.PanelPK
			}

			// go func() {
			// 	defer func() {
			// 		if r := recover(); r != nil {
			// 			logger.Errorf(
			// 				"PANIC ProcessPanelEvent panel=%s panic=%v\n%s",
			// 				d.PanelID,
			// 				r,
			// 				debug.Stack(),
			// 			)
			// 		}
			// 	}()

			// 	ProcessPanelEvent(s.pool, d.PanelID, cmd, d.SiaCode, alerttime, d.ClientID)
			// }()

			// Check if command exists in otherCommands
			if isExistInOtherCommands(d.SiaCode) {
				logger.Infof("Command %s exists in otherCommands", d.SiaCode)
				go handler.HandleOtherCommands(d, d.SiaCode, &panelLogData, s.pool, s.enqueueLog, s.panelSensorCache)
				return
			} else {
				if len(d.SiaCode) >= 32 {
					logger.Warnf("only digit siacode or Sensor Status string for panelID %s | SIAcode %s", d.PanelID, d.SiaCode)
					go s.enqueueLog(panelLogData)
					go handler.UpdateSensorStatus(s.pool, d, s.panelSensorCache)
					return

				}

				if d.SiaCode != "" {

					ctx := context.Background()

					count, err := s.zoneAlertCache.Increment(
						ctx,
						d.PanelID,
						d.ClientID,
						d.SiaCode,
					)

					if err != nil {

						logger.Errorf(
							"[ZONE-COUNT] panel=%s zone=%s err=%v",
							d.PanelID,
							d.SiaCode,
							err,
						)

					} else {

						if count > 13 {

							logger.Warnf(
								"[AUTO-BYPASS] panel=%s client=%s zone=%s count=%d",
								d.PanelID,
								d.ClientID,
								d.SiaCode,
								count,
							)

							func() {
								if len(d.SiaCode) < 6 {

									logger.Errorf(
										"invalid sia code=%s panel=%s",
										d.SiaCode,
										d.PanelID,
									)

									return
								}

								siaCode := fmt.Sprintf(
									"DCS033|W|%s|0",
									d.SiaCode[3:6],
								)

								cmd := parser.CreateCommands(
									siaCode,
									d,
								)

								reply, ok := utils.SendMsgToPanel(
									cmd,
									panelIP,
									"2300",
								)

								if !ok {

									logger.Errorf(
										"Bypass send failed panel=%s zone=%s reply=%s",
										d.PanelID,
										d.SiaCode,
										reply,
									)

								}

								// panelLogData.Msg = cmd
								// panelLogData.Response = reply
								// panelLogData.MsgFrom = constants.APPLICATION
								// panelLogData.MsgTo = constants.PANEL

								// panelLogData.MsgType = "BYPASS"

								// panelLogData.IsTicketCreated = false

								// panelLogData.TicketRemark = fmt.Sprintf(
								// 	"Zone %s auto bypassed after %d alerts in 1 hour",
								// 	d.SiaCode,
								// 	count,
								// )
								// go s.enqueueLog(panelLogData)

							}()

							return
						}
					}
				}

				// ctx := context.Background()

				// shouldCreate, _ := s.ShouldCreateTicket(
				// 	ctx,
				// 	d.PanelID,
				// 	d.ClientID,
				// 	d.SiaCode,
				// )

				// if !shouldCreate {
				// 	panelLogData.IsTicketCreated = false
				// 	panelLogData.TicketRemark = "Sensor Already in Alarm"
				// 	go s.enqueueLog(panelLogData)
				// 	return
				// }
				logger.Infof("Processing command %s for DB storage", d.SiaCode)

				response, err := handler.CreateTicket(d, d.SiaCode, alerttime.Format("2006-01-02 15:04:05"), s.cfg.DjangoHost)
				if err != nil {

					if response != nil {
						logger.Errorf(
							"Ticket Creation Failed for Panel :%q | Code: %s | APIError: %v | Error: %s",
							d.PanelID,
							d.SiaCode,
							response.Error,
							err,
						)
					} else {
						logger.Errorf(
							"Ticket Creation Failed for Panel :%q | Code: %s | Error: %s",
							d.PanelID,
							d.SiaCode,
							err,
						)
					}
					panelLogData.IsTicketCreated = false
					panelLogData.TicketRemark = fmt.Sprintf("Ticket Creation Failed: %v", err)
					go s.enqueueLog(panelLogData)
					go handler.UpdateSensorStatus(s.pool, d, s.panelSensorCache)
					return
				}
				logger.Infof("Ticket Created Successfully | Panel: %s | Message: %s", d.PanelID, response.Message)

				logger.Infof("Create ticket Message: %s for Panel: %s", response.Message, d.PanelID)
				isAutoClosed := false
				ticketCreated := response.IsTicketCreated
				ticketStatusLower := strings.ToLower(response.Message)

				for _, keyword := range []string{"internally", "duplicate", "already"} {
					if strings.Contains(ticketStatusLower, keyword) {
						isAutoClosed = true
						break
					}
				}
				panelLogData.IsTicketCreated = ticketCreated
				panelLogData.TicketRemark = response.Message
				panelLogData.IsAutoClosed = isAutoClosed
				go s.enqueueLog(panelLogData)
				go handler.UpdateSensorStatus(s.pool, d, s.panelSensorCache)
				return

			}

		}

	}
}

func (s *Server) ShouldCreateTicket(
	ctx context.Context,
	panelID,
	clientID,
	siaCode string,
) (bool, error) {

	if len(siaCode) < 6 {
		return true, nil
	}

	cfg, err := s.GetZoneConfig(
		ctx,
		panelID,
		clientID,
	)

	if err != nil {
		return true, err
	}

	zoneNo := siaCode[3:6]
	code := siaCode[:3]

	zoneCfg, ok := cfg[zoneNo]

	if !ok {

		logger.Warnf(
			"zone config missing panel=%s zone=%s",
			panelID,
			zoneNo,
		)

		return true, nil
	}

	// Restore
	if siaCode == zoneCfg.Restore {

		_ = s.panelZoneStateCache.Remove(
			ctx,
			panelID,
			clientID,
			zoneNo,
		)

		logger.Infof(
			"zone restored panel=%s zone=%s",
			panelID,
			zoneNo,
		)

		return true, nil
	}

	// Alarm
	if siaCode == zoneCfg.Alarm {

		active, err := s.panelZoneStateCache.Exists(
			ctx,
			panelID,
			clientID,
			zoneNo,
		)

		if err != nil {
			return true, err
		}

		if active {

			logger.Infof(
				"zone already active panel=%s zone=%s",
				panelID,
				zoneNo,
			)

			return false, nil
		}

		err = s.panelZoneStateCache.Set(
			ctx,
			panelID,
			clientID,
			zoneNo,
			code,
		)

		if err != nil {
			return true, err
		}

		return true, nil
	}

	return true, nil
}

func (s *Server) GetZoneConfig(
	ctx context.Context,
	panelID,
	clientID string,
) (map[string]models.ZoneConfig, error) {

	cfg, found, err := s.panelZoneDataCache.Get(
		ctx,
		panelID,
		clientID,
	)

	if err != nil {
		return nil, err
	}

	if found {
		return cfg, nil
	}

	cfg, err = db.LoadZoneConfig(
		ctx,
		s.pool,
		panelID,
		clientID,
	)

	if err != nil {
		return nil, err
	}

	if err := s.panelZoneDataCache.Set(
		ctx,
		panelID,
		clientID,
		cfg,
	); err != nil {

		logger.Errorf(
			"failed to cache zone config panel=%s client=%s err=%v",
			panelID,
			clientID,
			err,
		)
	}

	return cfg, nil
}

func (s *Server) Stop() {
	for _, ln := range s.listeners {
		_ = ln.Close()
	}
	s.pool.Close()
	logger.Info("Waiting for goroutines to exit...")
	s.wg.Wait()
}

func (s *Server) enqueueLog(log models.PanelLog) {
	if log.SensorName == "" {
		siaCode := strings.TrimSpace(log.SiaCode)

		// 🔹 Case 2: Generic sensor update (NBA001, NBR002, etc)
		if len(siaCode) >= 32 {
			log.SensorName = "All Sensor Info"
			log.MsgType = "All Sensor Info"

		} else if len(siaCode) == 12 && utils.IsAllDigits(siaCode) {
			log.SensorName = "Relay Info"
			log.MsgType = "Relay Info"
		} else if len(siaCode) == 6 {

			zone, status, err := handler.ParseGenericSIA(siaCode)
			if err != nil {
				logger.Warnf("Failed to parse generic SIA code %s: %v", siaCode, err)
			}

			panelMap, err := s.panelSensorCache.Get(log.PanelPK)
			if err != nil {
				logger.Warnf("[Enqueue Log] cache fetch failed panel=%s err=%v", log.PanelId, err)
			}

			sensorName := ""
			if panelMap != nil {
				if val, ok := panelMap[siaCode]; ok {
					sensorName = val
				}
			}

			logger.Infof(
				"[Enqueue Log] update | panel=%s zone=%s status=%s sensor=%s",
				log.PanelId,
				zone,
				status,
				sensorName,
			)
			log.SensorName = sensorName
		} else {

		}
	}

	select {
	case s.logQueue <- log:
		// queued successfully
	default:
		logger.Warn("Log queue full — dropping log")
	}
}

func (s *Server) enqueueUpdatePanel(panelUpdate models.PanelIPUpdate) {
	select {
	case s.updatePanelQueue <- panelUpdate:
		// queued successfully
	default:
		logger.Warn("updatePanelQueue queue full — dropping log")
	}
}

func (s *Server) enqueueHeartBeat(heartBeat models.PanelHeartBeat) {
	select {
	case s.panelHeartBeatQueue <- heartBeat:
		// queued successfully
	default:
		logger.Warn("panelHeartBeatQueue queue full — dropping log")
	}
}

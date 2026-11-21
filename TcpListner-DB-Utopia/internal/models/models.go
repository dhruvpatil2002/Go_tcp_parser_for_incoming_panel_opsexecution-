package models

import "time"

type PanelLog struct {
	PanelId         string
	ClientId        string
	IpAddress       string
	Rvalue          string
	Lvalue          string
	SiaCode         string
	MsgType         string
	Msg             string
	Response        string
	MsgFrom         string
	MsgTo           string
	AlertTime       time.Time
	ReceiveTime     time.Time
	IsTicketCreated bool
	TicketRemark    string
	IsAutoClosed    bool
	SensorName      string
	PanelPK         string
}

type PanelIPUpdate struct {
	PanelId  string
	ClientId string
	PanelIP  string
}

type PanelHeartBeat struct {
	PanelID     string
	ClientID    string
	PanelIP     string
	Source      string
	ReceiveTime time.Time
}

type PanelData struct {
	PanelPK  string
	PanelID  string
	ClientID string
	MacID    string
}

type TicketAPIRequest struct {
	PanelID   string `json:"panelid"`
	SIACode   string `json:"SIACode"`
	AlertTime string `json:"alerttime"`
	ClientID  string `json:"client_id"`
}

type TicketAPIResponse struct {
	Error           string `json:"error"`
	Message         string `json:"message"`
	IsTicketCreated bool   `json:"is_ticket_created"`
}

type ZoneConfig struct {
	Alarm   string
	Restore string
}

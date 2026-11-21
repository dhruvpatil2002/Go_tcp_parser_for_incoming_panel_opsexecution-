package constants

const (
	ArmCommand    = "NCL"
	DisarmCommand = "NOA"
)

const APPLICATION = "APPLICATION"
const PANEL = "PANEL"

type Source string

var HeartBeat Source = "TCP_HEARTBEAT"

// REDIS CONSTANTS KEYS
const (
	RedisPanelDataPrefix = "tcp_alert:panel_data:"
	RedisPanelPrefix     = "tcp_alert:panel:"
)

const (
	RedisZoneAlertCount = "zone_alert_count"
	RedisZoneState      = "zone_state"
	RedisZoneData       = "zone_data"
)

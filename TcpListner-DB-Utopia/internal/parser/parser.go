package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shehbaaz/db-utopia/internal/utils"
)

var siaRx = regexp.MustCompile(
	// `^` +
	// 	`([0-9A-F]{4})` + // CRC
	// 	`([0-9A-F]{4})` + // Length
	`"SIA-DCS"` +
		`(\d{4})R(\d{6})L(\d{6})` + // Seq, Receiver, Client
		`#([A-Z0-9]+)` + // PanelID
		`\[(.*?)\]` + // Payload
		`_(\d{2}:\d{2}:\d{2}),` + // Time
		`(\d{2}-\d{2}-\d{4})$`, // Date
)
var nullPattern = regexp.MustCompile(
	`"NULL"` +
		`(\d{4})` + // Seq
		`R(\d{6})` + // Receiver
		`L(\d{6})` + // Client
		`#([A-Z0-9]+)` + // PanelID
		`\[\]` + // Empty payload
		`_(\d{2}:\d{2}:\d{2},\d{2}-\d{2}-\d{4})$`, // Timestamp
)

type SIAData struct {
	Raw string
	// CRC      string
	// Length   string
	SeqNo    string
	Receiver string
	ClientID string
	PanelID  string
	Payload  string
	SiaCode  string
	TimeStr  string
	DateStr  string
	PanelIP  string
	PanelPK  string
}

type NULLData struct {
	SeqNo     string
	Receiver  string
	ClientID  string
	PanelID   string
	Timestamp time.Time
	PanelIP   string
}

type ZoneStatus struct {
	Zone       string
	Status     string
	SensorName string
	AlertTime  time.Time
}

func ParseSIA(cmd string) (*SIAData, bool) {
	cmd = strings.TrimSpace(cmd)
	cmd = strings.TrimPrefix(cmd, "\x0A")
	cmd = strings.TrimSuffix(cmd, "\x0D")

	m := siaRx.FindStringSubmatch(cmd)
	if m == nil {
		return nil, false
	}
	// parts := strings.Split(m[7], "|")
	parts := strings.Split(m[5], "|")
	if len(parts) < 2 {
		return nil, false
	}
	siaCode := strings.Split(parts[1], ".")[0]

	// return &SIAData{
	// 	Raw:      cmd,
	// 	CRC:      m[1],
	// 	Length:   m[2],
	// 	SeqNo:    m[3],
	// 	Receiver: m[4],
	// 	ClientID: m[5],
	// 	PanelID:  m[6],
	// 	Payload:  m[7],
	// 	SiaCode:  siaCode,
	// 	TimeStr:  m[8],
	// 	DateStr:  m[9],
	// }, true
	return &SIAData{
		Raw:      cmd,
		SeqNo:    m[1],
		Receiver: m[2],
		ClientID: m[3],
		PanelID:  m[4],
		Payload:  m[5],
		SiaCode:  siaCode,
		TimeStr:  m[6],
		DateStr:  m[7],
	}, true
}

func (s *SIAData) ExtractZoneStatuses() ([]ZoneStatus, bool) {
	parts := strings.Split(s.Payload, "|")

	// Must have at least: Panel | SIA | one zone
	if len(parts) < 3 {
		return nil, false
	}

	var zones []ZoneStatus

	for _, z := range parts[2:] {
		if len(z) < 2 {
			continue
		}

		zoneStr := z[:len(z)-1] // all except last char
		status := z[len(z)-1:]  // last char

		zones = append(zones, ZoneStatus{
			Zone:   zoneStr,
			Status: status,
		})
	}

	if len(zones) == 0 {
		return nil, false
	}

	return zones, true
}

func parseSIATimestamp(tsStr string) (time.Time, error) {
	tsStr = strings.TrimSpace(tsStr)

	parts := strings.Split(tsStr, ",")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid timestamp format: %q", tsStr)
	}

	timePart := parts[0] // "07:56:03"
	datePart := parts[1] // "04-17-2026"

	// Parse time
	t, err := time.Parse("15:04:05", timePart)
	if err != nil {
		return time.Time{}, fmt.Errorf("time parse error: %w", err)
	}

	// Parse date (try both formats)
	var d time.Time

	d, err = time.Parse("01-02-2006", datePart)

	if err != nil {
		return time.Time{}, fmt.Errorf("date parse error: %w", err)
	}

	// Combine
	return time.Date(
		d.Year(),
		d.Month(),
		d.Day(),
		t.Hour(),
		t.Minute(),
		t.Second(),
		0,
		utils.IST,
	), nil
}

// ParseNULL parses "NULL" messages and returns a structured object
func ParseNULL(cmd string) (*NULLData, bool) {
	cmd = strings.TrimSpace(cmd)
	cmd = strings.TrimPrefix(cmd, "\x0A")
	cmd = strings.TrimSuffix(cmd, "\x0D")
	m := nullPattern.FindStringSubmatch(cmd)
	if m == nil {
		return nil, false
	}

	ts, err := parseSIATimestamp(m[5])
	if err != nil {
		fmt.Printf("parse error: %v | value=%q\n", err, m[5])
		return nil, false
	}

	return &NULLData{
		SeqNo:     m[1],
		Receiver:  m[2],
		ClientID:  m[3],
		PanelID:   m[4],
		Timestamp: ts,
	}, true
}

// Timestamp parses TimeStr to a Go time.Time object
func (s *SIAData) Timestamp() (time.Time, error) {
	t, err := time.Parse("15:04:05", s.TimeStr)
	if err != nil {
		return time.Time{}, err
	}

	d, err := time.Parse("01-02-2006", s.DateStr)
	if err != nil {
		return time.Time{}, err
	}

	loc, _ := time.LoadLocation("Asia/Kolkata")
	return time.Date(
		d.Year(), d.Month(), d.Day(),
		t.Hour(), t.Minute(), t.Second(),
		0, loc,
	), nil
}

// GetChecksum computes CRC-16 and returns a formatted checksum + length string (like "XXXXYYYY")
func GetChecksum(command string) string {
	checkSum := getCheckSum(command)
	checkSumHex := fmt.Sprintf("%04X", checkSum)
	commandLengthHex := fmt.Sprintf("%04X", len(command))
	return checkSumHex + commandLengthHex
}

// getCheckSum calculates the CRC using Modbus-like algorithm

func getCheckSum(data string) uint16 {
	var crc uint16
	for _, c := range data {
		if c == 0 || c == 13 {
			break
		}
		crc = calculateCRC(crc, uint16(c))
	}
	return crc
}

// calculateCRC performs CRC-16 (Modbus variant) bitwise computation
func calculateCRC(crc, charPos uint16) uint16 {
	temp := charPos
	for i := 0; i < 8; i++ {
		temp ^= crc & 1
		crc >>= 1
		if temp&1 != 0 {
			crc ^= 0xA001
		}
		temp >>= 1
	}
	return crc
}

// GetAck generates a full ACK string (with checksum + content)
func (s *SIAData) GetACK() string {
	body := fmt.Sprintf(
		`"ACK"%sR%sL%s#%s[]`,
		s.SeqNo, s.Receiver, s.ClientID, s.PanelID,
	)

	crc := getCheckSum(body)
	length := fmt.Sprintf("%04X", len(body))

	return fmt.Sprintf("\x0A%04X%s%s\x0D", crc, length, body)
}

// GetNAK creates a full NAK message string from NULLData
func (n *NULLData) GetNAK() string {
	now := time.Now()
	nakTime := now.Format("15:04:05,01-02-2006")
	body := fmt.Sprintf(
		`"NAK"%sR%sL%s#%s[]_%s`,
		n.SeqNo, n.Receiver, n.ClientID, n.PanelID, nakTime,
	)

	crc := getCheckSum(body)
	length := fmt.Sprintf("%04X", len(body))

	return fmt.Sprintf("\x0A%04X%s%s\x0D", crc, length, body)
}

func (n *NULLData) GetACK() string {
	body := fmt.Sprintf(
		`"ACK"%sR%sL%s#%s[]`,
		n.SeqNo, n.Receiver, n.ClientID, n.PanelID,
	)

	crc := getCheckSum(body)
	length := fmt.Sprintf("%04X", len(body))

	return fmt.Sprintf("\x0A%04X%s%s\x0D", crc, length, body)
}

// GetNAK creates a full NAK message string from NULLData
func (s *SIAData) GetNAK() string {
	now := time.Now()
	nakTime := now.Format("15:04:05,01-02-2006")
	body := fmt.Sprintf(
		`"NAK"%sR%sL%s#%s[]_ %s`,
		s.SeqNo, s.Receiver, s.ClientID, s.PanelID, nakTime,
	)

	crc := getCheckSum(body)
	length := fmt.Sprintf("%04X", len(body))

	return fmt.Sprintf("\x0A%04X%s%s\x0D", crc, length, body)
}

func CreateCommands(siaCode string, data *SIAData) string {

	currentTime := time.Now().Format("15:04:05,02-01-2006")

	body := (`"SIA-DCS"` + "0001" + "R" + data.Receiver + "L" + data.ClientID +
		"#" + data.PanelID + "[#" + data.PanelID + "|" + siaCode + "]_" + currentTime)
	crc := getCheckSum(body)
	length := fmt.Sprintf("%04X", len(body))

	return fmt.Sprintf("\x0A%04X%s%s\x0D", crc, length, body)

}
  //<LF><CRC><LENGTH><MESSAGE><CR>
  //<LF>01AF002A"SIA-DCS"0001L0#1234[#1234|NBA001]<CR>
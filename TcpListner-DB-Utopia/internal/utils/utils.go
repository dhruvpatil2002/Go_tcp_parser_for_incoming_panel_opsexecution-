package utils

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/shehbaaz/db-utopia/internal/logger"
)

var IST, _ = time.LoadLocation("Asia/Kolkata")

func ReadPanelIP(conn net.Conn) (string, *bufio.Reader, error) {

	reader := bufio.NewReader(conn)

	remoteAddr := conn.RemoteAddr().String()

	if strings.Contains(remoteAddr, "10.80.8.246") || strings.Contains(remoteAddr, "10.80.8.245") {
		return remoteAddr, reader, nil
	}
	peek, err := reader.Peek(5)
	if err != nil {

		if err == io.EOF {

			logger.Warnf(
				"Client disconnected before data | remote=%s | error=%v",
				remoteAddr,
				err,
			)

			return "", reader, fmt.Errorf(
				"client disconnected before data | remote=%s",
				remoteAddr,
			)
			// return "", reader, nil
		}

		return "", reader, err
	}

	if string(peek) != "PROXY" {

		ip, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			return "", reader, err
		}

		return ip, reader, nil
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", reader, err
	}

	parts := strings.Fields(strings.TrimSpace(line))

	if len(parts) < 6 {
		return "", reader, fmt.Errorf("invalid proxy header")
	}

	return parts[2], reader, nil
}
func IsAllDigits(s string) bool {

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func SendMsgToPanel(msg, panelIp, panelPort string) (string, bool) {
	addr := net.JoinHostPort(panelIp, panelPort)

	conn, err := net.DialTimeout("tcp", addr, 35*time.Second)
	if err != nil {
		logger.Errorf("Panel connect failed | ip=%s port=%s err=%v", panelIp, panelPort, err)
		return "", false
	}
	defer func() {
		if err := conn.Close(); err != nil &&
			!strings.Contains(err.Error(), "use of closed network connection") {

			logger.Errorf("Failed to close connection | err=%v", err)
		}
	}()

	logger.Infof("Sending message | ip=%s port=%s msg=%s", panelIp, panelPort, msg)

	// WRITE
	if err := conn.SetWriteDeadline(time.Now().Add(35 * time.Second)); err != nil {
		logger.Errorf("Failed to set write deadline | err=%v", err)
		return "", false
	}
	if _, err := conn.Write([]byte(msg)); err != nil {
		logger.Errorf("Write failed | err=%v", err)
		return "", false
	}

	buf := make([]byte, 4096)

	// SAME AS OLD: 3 attempts
	for attempt := 1; attempt <= 3; attempt++ {

		logger.Infof("Waiting for ACK/NAK (attempt %d/3)", attempt)

		if err := conn.SetReadDeadline(time.Now().Add(35 * time.Second)); err != nil {
			logger.Errorf("Failed to set read deadline | err=%v", err)
			return "", false
		}

		n, err := conn.Read(buf)
		replyBytes := buf[:n]
		logger.Infof("Reply of panel ip: %s | HEX: % X", panelIp, replyBytes)
		if err != nil {
			logger.Errorf("Read failed | attempt=%d err=%v", attempt, err)
			continue
		}

		reply := string(buf[:n])
		logger.Infof("Received reply: %s", reply)

		if strings.Contains(reply, "ACK") {
			logger.Infof("ACK received from panel %s", panelIp)
			// time.Sleep(time.Duration(5) * time.Second)
			return reply, true
		}

		if strings.Contains(reply, "NAK") {
			logger.Infof("NAK received from panel %s", panelIp)

			// time.Sleep(time.Duration(5) * time.Second)
			return reply, false
		}

		logger.Warnf("No ACK/NAK in response (attempt %d)", attempt)
	}

	logger.Errorf("No valid ACK/NAK response after 3 attempts | ip=%s", panelIp)

	return "", false
}

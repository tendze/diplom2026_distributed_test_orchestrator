package agent

import (
	"fmt"
	"net"
	"strings"
)

func simplifyError(err error) string {
	if err == nil {
		return "OK"
	}

	if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
		return "Network Timeout"
	}

	s := err.Error()
	switch {
	case strings.Contains(s, "connection refused"):
		return "Conn Refused"
	case strings.Contains(s, "no such host"):
		return "DNS Error"
	case strings.Contains(s, "connection reset by peer"):
		return "Conn Reset"
	case strings.Contains(s, "EOF"):
		return "Server Closed Conn (EOF)"
	case strings.Contains(s, "context deadline exceeded"):
		return "Test Timeout"
	default:
		return fmt.Errorf("Other Network Error: %w", err).Error()
	}
}

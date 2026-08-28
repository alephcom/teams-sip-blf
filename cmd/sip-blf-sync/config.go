package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// parseContactPort parses SIP_CONTACT_PORT. Empty string means unset (returns 0, nil).
func parseContactPort(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	port, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid SIP_CONTACT_PORT %q: %w", s, err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("SIP_CONTACT_PORT out of range: %d", port)
	}
	return port, nil
}

// contactListenPort returns the port used for Contact/listen defaults (5060 if unset).
func contactListenPort(cfg sip.Config) int {
	if cfg.ContactPort > 0 {
		return cfg.ContactPort
	}
	return 5060
}

// defaultListenAddr returns the default bind address for the SIP server.
// Uses ContactPort when set; otherwise 5060. Binds 0.0.0.0 when ContactIP was a
// STUN sentinel or ContactPort is set (multi-instance / NAT-friendly).
func defaultListenAddr(cfg sip.Config) string {
	port := contactListenPort(cfg)
	if cfg.ContactPort != 0 || sip.IsContactSentinel(cfg.ContactIP) {
		return net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
	}
	return net.JoinHostPort(cfg.ContactIP, strconv.Itoa(port))
}

// listenPortFromAddr extracts the port from a host:port listen address.
func listenPortFromAddr(addr string) (int, error) {
	_, portStr, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portStr)
}

package sip

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/ccding/go-stun/stun"
)

const defaultSTUNPort = 19302 // Google STUN; standard is 3478

// DiscoverPublicAddress tries each STUN server in order using a simple binding
// request (RFC 5389) and returns the public (mapped) IP and port.
func DiscoverPublicAddress(servers []string, log *slog.Logger) (ip string, port int, err error) {
	if len(servers) == 0 {
		return "", 0, fmt.Errorf("no STUN servers configured")
	}
	var lastErr error
	var tried []string
	for _, srv := range servers {
		srv = strings.TrimSpace(srv)
		if srv == "" {
			continue
		}
		addr := normalizeSTUNAddr(srv)
		ip, port, err = discoverOne(addr)
		if err != nil {
			lastErr = err
			tried = append(tried, fmt.Sprintf("%s: %v", addr, err))
			if log != nil {
				log.Warn("STUN attempt failed", "server", addr, "error", err)
			}
			continue
		}
		if log != nil {
			log.Info("STUN discovery succeeded", "server", addr, "public", net.JoinHostPort(ip, strconv.Itoa(port)))
		}
		return ip, port, nil
	}
	msg := "all STUN servers failed"
	if len(tried) > 0 {
		msg = fmt.Sprintf("%s (tried: %s)", msg, strings.Join(tried, "; "))
		if log != nil {
			log.Error("STUN discovery failed", "servers_tried", tried, "last_error", lastErr)
		}
	}
	return "", 0, fmt.Errorf("%s", msg)
}

func normalizeSTUNAddr(srv string) string {
	host, portStr := srv, ""
	if idx := strings.LastIndex(srv, ":"); idx > 0 {
		host = srv[:idx]
		portStr = srv[idx+1:]
	}
	portNum := defaultSTUNPort
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			portNum = p
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(portNum))
}

// IsContactSentinel reports whether contactIP is a sentinel value that requires STUN.
// Used to avoid using the literal "stun" or "auto" as a hostname.
func IsContactSentinel(contactIP string) bool {
	c := strings.TrimSpace(strings.ToLower(contactIP))
	return c == "" || c == "auto" || c == "stun"
}

// ResolveContactIfNeeded runs STUN discovery when cfg.ContactIP is empty, "auto", or "stun",
// and sets cfg.ContactIP and cfg.ContactPort to the public address. Returns nil if no resolution needed or success.
func ResolveContactIfNeeded(cfg *Config, log *slog.Logger) error {
	if !IsContactSentinel(cfg.ContactIP) {
		return nil
	}
	if len(cfg.STUNServers) == 0 {
		return fmt.Errorf("STUN requested but no STUN_SERVERS configured")
	}
	ip, port, err := DiscoverPublicAddress(cfg.STUNServers, log)
	if err != nil {
		return err
	}
	cfg.ContactIP = ip
	cfg.ContactPort = port
	return nil
}

func discoverOne(serverAddr string) (ip string, port int, err error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return "", 0, err
	}
	defer conn.Close()

	client := stun.NewClientWithConnection(conn)
	client.SetServerAddr(serverAddr)
	host, err := client.Keepalive()
	if err != nil {
		return "", 0, err
	}
	if host == nil {
		return "", 0, fmt.Errorf("no mapped address in STUN response")
	}
	return host.IP(), int(host.Port()), nil
}

package main

import (
	"os"

	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// defaultListenAddr returns the default bind address for the SIP server. When
// ContactPort is set (STUN was used) or ContactIP is a sentinel (auto/stun/empty),
// we bind to 0.0.0.0:5060 so we never try to resolve "stun" as a hostname.
func defaultListenAddr(cfg sip.Config) string {
	if cfg.ContactPort != 0 || sip.IsContactSentinel(cfg.ContactIP) {
		return "0.0.0.0:5060"
	}
	return cfg.ContactIP + ":5060"
}

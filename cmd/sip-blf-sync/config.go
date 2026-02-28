package main

import (
	"encoding/json"
	"os"

	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

// ExtensionEntry is one row from extensions.json.
type ExtensionEntry struct {
	Extension string `json:"extension"`
	Email     string `json:"email"`
}

func loadExtensions(path string) ([]ExtensionEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []ExtensionEntry
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

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

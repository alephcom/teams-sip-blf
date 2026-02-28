package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

// ExtensionEntry is one row from extensions.json or extensions.csv.
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

// loadExtensionsCSV reads extension,email rows from a CSV file. Optional header row
// "extension,email" (case-insensitive) is detected and skipped. Spaces are trimmed; empty rows skipped.
func loadExtensionsCSV(path string) ([]ExtensionEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var list []ExtensionEntry
	for i, rec := range records {
		if len(rec) < 2 {
			continue
		}
		ext := strings.TrimSpace(rec[0])
		email := strings.TrimSpace(rec[1])
		if ext == "" && email == "" {
			continue
		}
		// Skip header row if present
		if i == 0 && strings.EqualFold(ext, "extension") && strings.EqualFold(email, "email") {
			continue
		}
		list = append(list, ExtensionEntry{Extension: ext, Email: email})
	}
	return list, nil
}

// loadExtensionsFromPath loads extensions from the given path. If the path exists, it is loaded as JSON
// (unless it ends in .csv, then as CSV). If the path does not exist and it ends in .json, the same path
// with .json replaced by .csv is tried as CSV. Returns the list, the path actually loaded from, and an error if none.
func loadExtensionsFromPath(path string) ([]ExtensionEntry, string, error) {
	if _, err := os.Stat(path); err == nil {
		if strings.HasSuffix(path, ".csv") {
			list, err := loadExtensionsCSV(path)
			return list, path, err
		}
		list, err := loadExtensions(path)
		return list, path, err
	}
	if strings.HasSuffix(path, ".json") {
		csvPath := strings.TrimSuffix(path, ".json") + ".csv"
		if _, err := os.Stat(csvPath); err == nil {
			list, err := loadExtensionsCSV(csvPath)
			return list, csvPath, err
		}
		return nil, "", errors.New("extensions file not found: tried " + path + " and " + csvPath)
	}
	return nil, "", errors.New("extensions file not found: " + path)
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

package extensions

import (
	"os"
	"strings"
)

const generalSection = "general"

// LoadVoicemail reads extension and email from an Asterisk voicemail.conf.
// Skips [general]. Mailbox lines: extension=password,name,email,...
// Uses the third field as email (first address if pipe-separated).
func LoadVoicemail(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var list []Entry
	seen := make(map[string]bool)
	var section string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if section == generalSection {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		parts := strings.Split(value, ",")
		if len(parts) < 3 {
			continue
		}
		emailField := strings.TrimSpace(parts[2])
		if emailField == "" || !strings.Contains(emailField, "@") {
			continue
		}
		email := strings.TrimSpace(strings.Split(emailField, "|")[0])
		if email == "" {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		list = append(list, Entry{Extension: key, Email: email})
	}
	return list, nil
}

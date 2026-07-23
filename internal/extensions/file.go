package extensions

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// LoadJSON reads [{extension,email}, ...] from a JSON file.
func LoadJSON(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []Entry
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return filterEmptyEmail(list), nil
}

// LoadCSV reads extension,email rows. Optional header row is skipped.
func LoadCSV(path string) ([]Entry, error) {
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
	var list []Entry
	for i, rec := range records {
		if len(rec) < 2 {
			continue
		}
		ext := strings.TrimSpace(rec[0])
		email := strings.TrimSpace(rec[1])
		if ext == "" && email == "" {
			continue
		}
		if i == 0 && strings.EqualFold(ext, "extension") && strings.EqualFold(email, "email") {
			continue
		}
		list = append(list, Entry{Extension: ext, Email: email})
	}
	return filterEmptyEmail(list), nil
}

// LoadFromPath loads JSON (or CSV if path ends in .csv). If a .json path is missing,
// the sibling .csv path is tried.
func LoadFromPath(path string) ([]Entry, string, error) {
	if _, err := os.Stat(path); err == nil {
		if strings.HasSuffix(path, ".csv") {
			list, err := LoadCSV(path)
			return list, path, err
		}
		list, err := LoadJSON(path)
		return list, path, err
	}
	if strings.HasSuffix(path, ".json") {
		csvPath := strings.TrimSuffix(path, ".json") + ".csv"
		if _, err := os.Stat(csvPath); err == nil {
			list, err := LoadCSV(csvPath)
			return list, csvPath, err
		}
		return nil, "", errors.New("extensions file not found: tried " + path + " and " + csvPath)
	}
	return nil, "", errors.New("extensions file not found: " + path)
}

func filterEmptyEmail(list []Entry) []Entry {
	out := make([]Entry, 0, len(list))
	for _, e := range list {
		e.Extension = strings.TrimSpace(e.Extension)
		e.Email = strings.TrimSpace(e.Email)
		if e.Extension == "" || e.Email == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

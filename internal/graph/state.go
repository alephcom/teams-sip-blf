package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// SessionState persists extension -> sessionId (UUID) for Graph presence sessions.
type SessionState struct {
	mu     sync.RWMutex
	path   string
	ByExt  map[string]string // extension -> sessionId UUID
}

// LoadSessionState reads the state file and returns a SessionState. If the file
// does not exist, creates it with an empty map. If the file is empty, returns state with empty map.
func LoadSessionState(path string) (*SessionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			s := &SessionState{path: path, ByExt: make(map[string]string)}
			if err := s.save(); err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, err
	}
	var byExt map[string]string
	if len(data) == 0 {
		byExt = make(map[string]string)
	} else {
		if err := json.Unmarshal(data, &byExt); err != nil {
			return nil, err
		}
		if byExt == nil {
			byExt = make(map[string]string)
		}
	}
	return &SessionState{path: path, ByExt: byExt}, nil
}

// GetSessionID returns the session ID for the extension, or "" if not set.
func (s *SessionState) GetSessionID(extension string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ByExt[extension]
}

// SetSessionID sets the session ID for the extension and persists to file.
func (s *SessionState) SetSessionID(extension, sessionID string) error {
	s.mu.Lock()
	s.ByExt[extension] = sessionID
	s.mu.Unlock()
	return s.save()
}

func (s *SessionState) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.ByExt, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	return os.WriteFile(s.path, data, 0600)
}

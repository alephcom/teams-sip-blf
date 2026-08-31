package sip

import (
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
)

const (
	defaultExpires   = 3600 * time.Second
	minExpires       = 60 * time.Second
	refreshNumerator = 4
	refreshDenom     = 5 // 80% of granted lifetime
)

// parseExpires reads the granted lifetime from a SIP response.
// Prefers the Expires header; falls back to Contact ;expires= parameter.
// Returns fallback when missing/invalid. Clamps to minExpires.
func parseExpires(res *sip.Response, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = defaultExpires
	}
	if res == nil {
		return clampExpires(fallback)
	}
	if h := res.GetHeader("Expires"); h != nil {
		if d, ok := parseExpiresSeconds(h.Value()); ok {
			return clampExpires(d)
		}
	}
	if h := res.GetHeader("Contact"); h != nil {
		if d, ok := parseContactExpires(h.Value()); ok {
			return clampExpires(d)
		}
	}
	return clampExpires(fallback)
}

func parseExpiresSeconds(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	sec, err := strconv.Atoi(s)
	if err != nil || sec < 0 {
		return 0, false
	}
	return time.Duration(sec) * time.Second, true
}

// parseContactExpires extracts ;expires=N from a Contact header value.
func parseContactExpires(contact string) (time.Duration, bool) {
	lower := strings.ToLower(contact)
	idx := strings.Index(lower, "expires=")
	if idx < 0 {
		return 0, false
	}
	rest := contact[idx+len("expires="):]
	end := len(rest)
	for i, r := range rest {
		if r == ';' || r == ',' || r == '>' || r == ' ' {
			end = i
			break
		}
	}
	return parseExpiresSeconds(rest[:end])
}

func clampExpires(d time.Duration) time.Duration {
	if d < minExpires {
		return minExpires
	}
	return d
}

// RefreshDelay returns when to refresh before expiry (80% of granted, floored at minExpires).
func RefreshDelay(granted time.Duration) time.Duration {
	granted = clampExpires(granted)
	d := granted * refreshNumerator / refreshDenom
	return clampExpires(d)
}

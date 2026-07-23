package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LoadFromURL GETs a JSON array of {extension,email} with optional Bearer token.
// Entries with empty email are skipped.
func LoadFromURL(ctx context.Context, url, token string) ([]Entry, error) {
	return LoadFromURLWithClient(ctx, http.DefaultClient, url, token)
}

// LoadFromURLWithClient is LoadFromURL with a custom HTTP client (for tests).
func LoadFromURLWithClient(ctx context.Context, client *http.Client, url, token string) ([]Entry, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if t := strings.TrimSpace(token); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("extensions URL: HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var list []Entry
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("extensions URL: decode JSON: %w", err)
	}
	return filterEmptyEmail(list), nil
}

// StartRefresh runs LoadFromURL on interval and calls onUpdate with the new list.
// Stops when ctx is cancelled. interval must be > 0.
func StartRefresh(ctx context.Context, url, token string, interval time.Duration, onUpdate func([]Entry), logf func(string, ...any)) {
	if interval <= 0 || onUpdate == nil {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				list, err := LoadFromURL(ctx, url, token)
				if err != nil {
					if logf != nil {
						logf("extensions URL refresh failed: %v", err)
					}
					continue
				}
				onUpdate(list)
				if logf != nil {
					logf("extensions URL refreshed: count=%d", len(list))
				}
			}
		}
	}()
}

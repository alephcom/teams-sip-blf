package extensions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pull_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"extension":"1001","email":"a@example.com"},
			{"extension":"1002","email":""},
			{"extension":"1003","email":"c@example.com"}
		]`))
	}))
	defer srv.Close()

	list, err := LoadFromURL(context.Background(), srv.URL, "pull_secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2", len(list))
	}
	if list[0].Extension != "1001" || list[0].Email != "a@example.com" {
		t.Errorf("got %+v", list[0])
	}
	if list[1].Extension != "1003" {
		t.Errorf("got %+v", list[1])
	}
}

func TestLoadFromURLUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	}))
	defer srv.Close()

	_, err := LoadFromURL(context.Background(), srv.URL, "bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadVoicemail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "voicemail.conf")
	content := `[general]
format=wav

[default]
1001=1234,Alice,alice@example.com
1002=1234,Bob,bob@example.com|other@example.com
1003=1234,NoEmail,
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := LoadVoicemail(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2", len(list))
	}
	if list[1].Email != "bob@example.com" {
		t.Errorf("pipe email = %q", list[1].Email)
	}
}

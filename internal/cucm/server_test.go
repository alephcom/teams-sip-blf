package cucm

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darrenwiebe/teams_freepbx/internal/blf"
)

func TestParseState(t *testing.T) {
	tests := []struct {
		in   string
		want blf.State
		ok   bool
	}{
		{"idle", blf.StateIdle, true},
		{"RINGING", blf.StateRinging, true},
		{" busy ", blf.StateBusy, true},
		{"held", blf.StateUnknown, false},
		{"", blf.StateUnknown, false},
	}
	for _, tt := range tests {
		got, err := ParseState(tt.in)
		if tt.ok {
			if err != nil {
				t.Errorf("ParseState(%q): unexpected error %v", tt.in, err)
				continue
			}
			if got != tt.want {
				t.Errorf("ParseState(%q) = %q, want %q", tt.in, got, tt.want)
			}
		} else if err == nil {
			t.Errorf("ParseState(%q): expected error", tt.in)
		}
	}
}

func TestHandleLineState(t *testing.T) {
	var gotExt string
	var gotState blf.State
	s := NewServer(Config{Token: "secret"}, func(ext string, state blf.State) {
		gotExt = ext
		gotState = state
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/line-state", bytes.NewBufferString(`{"extension":"1001","state":"busy"}`))
	req.Header.Set("X-CUCM-Token", "secret")
	rr := httptest.NewRecorder()
	s.handleLineState(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status %d, want %d", rr.Code, http.StatusNoContent)
	}
	if gotExt != "1001" || gotState != blf.StateBusy {
		t.Fatalf("handler got %q %q, want 1001 busy", gotExt, gotState)
	}
}

func TestHandleLineStateUnauthorized(t *testing.T) {
	s := NewServer(Config{Token: "secret"}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/line-state", bytes.NewBufferString(`{"extension":"1001","state":"idle"}`))
	rr := httptest.NewRecorder()
	s.handleLineState(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandleLineStateBadJSON(t *testing.T) {
	s := NewServer(Config{}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/line-state", bytes.NewBufferString(`not-json`))
	rr := httptest.NewRecorder()
	s.handleLineState(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestServerStartClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	s := NewServer(Config{ListenAddr: addr}, nil)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Close()

	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status %d", resp.StatusCode)
	}
}

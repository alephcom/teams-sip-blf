package sip

import (
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func testResponse(headers map[string]string) *sip.Response {
	uri := sip.Uri{}
	_ = sip.ParseUri("sip:user@pbx.example.com", &uri)
	req := sip.NewRequest(sip.REGISTER, uri)
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	for name, val := range headers {
		res.AppendHeader(sip.NewHeader(name, val))
	}
	return res
}

func TestParseExpires_Header(t *testing.T) {
	res := testResponse(map[string]string{"Expires": "1800"})
	got := parseExpires(res, defaultExpires)
	if got != 1800*time.Second {
		t.Fatalf("got %v, want 1800s", got)
	}
}

func TestParseExpires_ContactParam(t *testing.T) {
	res := testResponse(map[string]string{
		"Contact": `<sip:user@1.2.3.4:5060>;expires=900`,
	})
	got := parseExpires(res, defaultExpires)
	if got != 900*time.Second {
		t.Fatalf("got %v, want 900s", got)
	}
}

func TestParseExpires_HeaderPrefersOverContact(t *testing.T) {
	res := testResponse(map[string]string{
		"Expires": "1200",
		"Contact": `<sip:user@1.2.3.4>;expires=600`,
	})
	got := parseExpires(res, defaultExpires)
	if got != 1200*time.Second {
		t.Fatalf("got %v, want 1200s", got)
	}
}

func TestParseExpires_MissingUsesFallback(t *testing.T) {
	res := testResponse(nil)
	got := parseExpires(res, 2400*time.Second)
	if got != 2400*time.Second {
		t.Fatalf("got %v, want 2400s", got)
	}
}

func TestParseExpires_InvalidUsesFallback(t *testing.T) {
	res := testResponse(map[string]string{"Expires": "not-a-number"})
	got := parseExpires(res, 1500*time.Second)
	if got != 1500*time.Second {
		t.Fatalf("got %v, want 1500s", got)
	}
}

func TestParseExpires_NilResponseUsesFallback(t *testing.T) {
	got := parseExpires(nil, 2000*time.Second)
	if got != 2000*time.Second {
		t.Fatalf("got %v, want 2000s", got)
	}
}

func TestParseExpires_ClampsBelowFloor(t *testing.T) {
	res := testResponse(map[string]string{"Expires": "10"})
	got := parseExpires(res, defaultExpires)
	if got != minExpires {
		t.Fatalf("got %v, want minExpires %v", got, minExpires)
	}
}

func TestParseContactExpires(t *testing.T) {
	cases := []struct {
		contact string
		want    time.Duration
		ok      bool
	}{
		{`<sip:u@h>;expires=3600`, 3600 * time.Second, true},
		{`<sip:u@h:5071>;expires=60;q=1.0`, 60 * time.Second, true},
		{`sip:u@h`, 0, false},
		{`<sip:u@h>;expires=bad`, 0, false},
	}
	for _, tc := range cases {
		got, ok := parseContactExpires(tc.contact)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseContactExpires(%q) = (%v, %v), want (%v, %v)", tc.contact, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRefreshDelay(t *testing.T) {
	got := RefreshDelay(3600 * time.Second)
	want := 2880 * time.Second // 80% of 3600
	if got != want {
		t.Fatalf("RefreshDelay(3600s) = %v, want %v", got, want)
	}
}

func TestRefreshDelay_Floor(t *testing.T) {
	got := RefreshDelay(60 * time.Second) // 80% = 48s → clamp to 60s
	if got != minExpires {
		t.Fatalf("RefreshDelay(60s) = %v, want %v", got, minExpires)
	}
}

func TestRefreshDelay_ZeroFallsBackViaClamp(t *testing.T) {
	got := RefreshDelay(0)
	if got != minExpires {
		t.Fatalf("RefreshDelay(0) = %v, want %v", got, minExpires)
	}
}

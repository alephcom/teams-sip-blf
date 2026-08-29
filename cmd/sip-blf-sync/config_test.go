package main

import (
	"testing"

	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

func TestParseContactPort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", 0, false},
		{"  ", 0, false},
		{"5071", 5071, false},
		{"1", 1, false},
		{"65535", 65535, false},
		{"0", 0, true},
		{"65536", 0, true},
		{"abc", 0, true},
	}
	for _, tc := range cases {
		got, err := parseContactPort(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseContactPort(%q): want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseContactPort(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseContactPort(%q)=%d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestDefaultListenAddr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		cfg      sip.Config
		stunUsed bool
		want     string
	}{
		{
			name: "plain contact ip defaults to 5060 on that ip",
			cfg:  sip.Config{ContactIP: "10.81.223.208"},
			want: "10.81.223.208:5060",
		},
		{
			name: "contact port binds all interfaces on that port",
			cfg:  sip.Config{ContactIP: "10.81.223.208", ContactPort: 5071},
			want: "0.0.0.0:5071",
		},
		{
			name: "contact port 5060 still uses all interfaces",
			cfg:  sip.Config{ContactIP: "10.81.223.208", ContactPort: 5060},
			want: "0.0.0.0:5060",
		},
		{
			name: "stun sentinel before resolve uses all interfaces",
			cfg:  sip.Config{ContactIP: "auto"},
			want: "0.0.0.0:5060",
		},
		{
			name:     "stun mapped port is not used as a local bind port",
			cfg:      sip.Config{ContactIP: "203.0.113.10", ContactPort: 49152},
			stunUsed: true,
			want:     "0.0.0.0:5060",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := defaultListenAddr(tc.cfg, tc.stunUsed); got != tc.want {
				t.Fatalf("defaultListenAddr()=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestListenPortFromAddr(t *testing.T) {
	t.Parallel()
	got, err := listenPortFromAddr("0.0.0.0:5071")
	if err != nil {
		t.Fatal(err)
	}
	if got != 5071 {
		t.Fatalf("got %d", got)
	}
}

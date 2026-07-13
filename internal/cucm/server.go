package cucm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/darrenwiebe/teams_freepbx/internal/blf"
	"github.com/darrenwiebe/teams_freepbx/internal/provider"
)

// Ensure Server implements provider.Provider.
var _ provider.Provider = (*Server)(nil)

const maxBodyBytes = 1 << 20 // 1 MiB

// Config holds CUCM event ingress settings.
type Config struct {
	ListenAddr string // e.g. 127.0.0.1:8090
	Token      string // optional; if set, require X-CUCM-Token header
}

// lineStateRequest is the JSON body from the JTAPI sidecar.
type lineStateRequest struct {
	Extension string `json:"extension"`
	State     string `json:"state"`
}

// Server receives line-state POSTs from the CUCM JTAPI sidecar.
type Server struct {
	cfg     Config
	handler provider.Handler
	log     *slog.Logger
	httpSrv *http.Server
}

// NewServer creates a CUCM event ingress server.
func NewServer(cfg Config, onLineState provider.Handler) *Server {
	return &Server{
		cfg:     cfg,
		handler: onLineState,
		log:     slog.Default().With("component", "cucm"),
	}
}

// ParseState maps a sidecar state string to blf.State.
func ParseState(s string) (blf.State, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "idle":
		return blf.StateIdle, nil
	case "ringing":
		return blf.StateRinging, nil
	case "busy":
		return blf.StateBusy, nil
	default:
		return blf.StateUnknown, fmt.Errorf("unknown state %q", s)
	}
}

// Start begins listening for HTTP line-state events. Call Close to shut down.
func (s *Server) Start(ctx context.Context) error {
	_ = ctx
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/line-state", s.handleLineState)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	s.httpSrv = &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.ListenAddr, err)
	}
	s.log.Info("listening for CUCM line-state events", "addr", s.cfg.ListenAddr)

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error("http server", "error", err)
		}
	}()
	return nil
}

// Close shuts down the HTTP server.
func (s *Server) Close() error {
	if s.httpSrv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) handleLineState(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Token != "" {
		if r.Header.Get("X-CUCM-Token") != s.cfg.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req lineStateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Extension = strings.TrimSpace(req.Extension)
	if req.Extension == "" {
		http.Error(w, "extension required", http.StatusBadRequest)
		return
	}
	state, err := ParseState(req.State)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.handler != nil {
		s.handler(req.Extension, state)
	}
	w.WriteHeader(http.StatusNoContent)
}

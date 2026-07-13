package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/darrenwiebe/teams_freepbx/internal/sip"
)

// SIPConfig holds SIP provider settings.
type SIPConfig struct {
	sip.Config
	ListenAddr string
}

// SIP wraps the existing SIP BLF client as a Provider.
type SIP struct {
	client     *sip.Client
	cfg        SIPConfig
	listenAddr string
	log        *slog.Logger
	cancel     context.CancelFunc
}

// NewSIP creates a SIP line-state provider.
func NewSIP(cfg SIPConfig, extensions []string, onLineState Handler) (*SIP, error) {
	client, err := sip.NewClient(cfg.Config, extensions, sip.BLFHandler(onLineState))
	if err != nil {
		return nil, err
	}
	return &SIP{
		client:     client,
		cfg:        cfg,
		listenAddr: cfg.ListenAddr,
		log:        slog.Default().With("component", "provider-sip"),
	}, nil
}

// Start listens for NOTIFY, registers, and subscribes to BLF.
func (p *SIP) Start(ctx context.Context) error {
	listenCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	go func() {
		if err := p.client.ListenAndServe(listenCtx, p.cfg.Transport, p.listenAddr); err != nil && listenCtx.Err() == nil {
			p.log.Error("sip server", "error", err)
		}
	}()

	if err := p.client.Register(ctx); err != nil {
		cancel()
		return fmt.Errorf("register: %w", err)
	}
	if err := p.client.Subscribe(ctx); err != nil {
		cancel()
		return fmt.Errorf("subscribe: %w", err)
	}
	return nil
}

// Close shuts down the SIP client.
func (p *SIP) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	return p.client.Close()
}

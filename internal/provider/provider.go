package provider

import (
	"context"

	"github.com/darrenwiebe/teams_freepbx/internal/blf"
)

// Handler is called when a line-state change is received (extension, state).
type Handler func(extension string, state blf.State)

// Provider supplies line state from a backend (SIP BLF, CUCM CTI, etc.).
type Provider interface {
	// Start begins observing line state. It returns once the provider is running
	// (or fails to start). Call Close to shut down.
	Start(ctx context.Context) error
	Close() error
}

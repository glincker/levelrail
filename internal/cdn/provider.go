// Package cdn holds what every edge provider shares: the provider contract,
// the purge history and the per-step setup result, so callers never branch on
// which CDN sits in front of an app.
package cdn

import (
	"context"
	"sync"
)

// ProviderCloudflare is the name the Cloudflare adapter reports.
const ProviderCloudflare = "cloudflare"

// Provider is the edge operations Levelrail needs from a CDN.
type Provider interface {
	// Name is the stable provider id shown in status and history.
	Name() string
	// PurgeHosts clears the edge cache for every URL under each host in zone.
	PurgeHosts(ctx context.Context, zone string, hosts []string) error
	// SSLMode reads how the edge talks to the origin: off, flexible, full or strict.
	SSLMode(ctx context.Context, zone string) (string, error)
}

// Fake is an in-memory Provider for tests and for providers not yet wired.
type Fake struct {
	ID       string
	Mode     string
	PurgeErr error
	ModeErr  error

	mu     sync.Mutex
	purged [][]string
}

// Name implements Provider.
func (f *Fake) Name() string {
	if f.ID == "" {
		return "fake"
	}
	return f.ID
}

// PurgeHosts records the hosts and returns PurgeErr.
func (f *Fake) PurgeHosts(_ context.Context, _ string, hosts []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purged = append(f.purged, append([]string(nil), hosts...))
	return f.PurgeErr
}

// SSLMode returns Mode, or ModeErr.
func (f *Fake) SSLMode(context.Context, string) (string, error) {
	if f.ModeErr != nil {
		return "", f.ModeErr
	}
	return f.Mode, nil
}

// Purges returns the host batches purged so far.
func (f *Fake) Purges() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.purged...)
}

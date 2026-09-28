package models

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// HostResolver computes the hostnames a model is served on: its
// configured domain, or the zero-config fallback derived from the
// control plane's public host.
type HostResolver struct {
	publicHost string
	fallback   func(publicHost, label string) (string, bool)
}

// NewHostResolver builds a HostResolver. fallback may be nil (no
// fallback hostnames).
func NewHostResolver(publicHost string, fallback func(publicHost, label string) (string, bool)) *HostResolver {
	return &HostResolver{publicHost: publicHost, fallback: fallback}
}

// Hosts returns the hostnames serving m.
func (h *HostResolver) Hosts(m store.Model) []string {
	if m.Domain != "" {
		return []string{m.Domain}
	}
	if h != nil && h.fallback != nil {
		if d, ok := h.fallback(h.publicHost, "model-"+m.Name); ok {
			return []string{d}
		}
	}
	return nil
}

// BaseURL returns the OpenAI-compatible base URL for m, or empty when it
// has no reachable hostname.
func (h *HostResolver) BaseURL(m store.Model) string {
	hosts := h.Hosts(m)
	if len(hosts) == 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/v1", hosts[0])
}

// HostLister is what the ingress controller needs to route model hosts.
type HostLister struct {
	Store GatewayStore
	Hosts *HostResolver
}

// ModelHosts returns every hostname of every live model.
func (l HostLister) ModelHosts(ctx context.Context) ([]string, error) {
	list, err := l.Store.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list model hosts: %w", err)
	}
	var out []string
	for _, m := range list {
		if !m.Deleting {
			out = append(out, l.Hosts.Hosts(m)...)
		}
	}
	return out, nil
}

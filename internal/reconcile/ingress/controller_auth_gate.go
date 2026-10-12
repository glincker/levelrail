package ingress

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuthGateStore is optional on ServiceStore: a store without it (most test
// fakes) routes every domain without an auth gate.
type AuthGateStore interface {
	ListDomainAuthGates(ctx context.Context) ([]store.DomainAuthGate, error)
}

// applyAuthGates attaches each enabled domain's auth gate to its route. A
// gate that no longer validates leaves its host unrouted: fail closed.
func (c *Controller) applyAuthGates(ctx context.Context, routes []ingress.ProxyRoute) ([]ingress.ProxyRoute, error) {
	gs, ok := c.store.(AuthGateStore)
	if !ok {
		return routes, nil
	}
	rows, err := gs.ListDomainAuthGates(ctx)
	if err != nil {
		return routes, err
	}
	gates := authGatesByDomain(ctx, c.logger, rows, os.Getenv)
	if len(gates) == 0 {
		return routes, nil
	}
	return attachAuthGates(routes, gates), nil
}

// authGatesByDomain maps every enabled row to its gate; a nil value marks a
// gated domain whose stored config is invalid.
func authGatesByDomain(ctx context.Context, logger *slog.Logger, rows []store.DomainAuthGate, getenv func(string) string) map[string]*ingress.AuthGate {
	defaultTimeout, err := ingress.AuthGateTimeoutFromEnv(getenv)
	if err != nil {
		logger.WarnContext(ctx, "ingress: ignoring invalid auth gate timeout", slog.String("error", err.Error()))
	}
	gates := map[string]*ingress.AuthGate{}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		g := &ingress.AuthGate{URL: row.AuthURL, CheckPath: row.CheckPath, CopyHeaders: row.CopyHeaders, Timeout: defaultTimeout}
		if row.TimeoutSeconds > 0 {
			g.Timeout = time.Duration(row.TimeoutSeconds) * time.Second
		}
		if err := g.Validate(); err != nil {
			logger.WarnContext(ctx, "ingress: auth gate invalid, domain left unrouted",
				slog.String("domain", row.Domain), slog.String("error", err.Error()))
			gates[row.Domain] = nil
			continue
		}
		gates[row.Domain] = g
	}
	return gates
}

// attachAuthGates gives every gated host its own route so a gate never
// spills onto a sibling host, and drops hosts whose gate is invalid.
func attachAuthGates(routes []ingress.ProxyRoute, gates map[string]*ingress.AuthGate) []ingress.ProxyRoute {
	out := make([]ingress.ProxyRoute, 0, len(routes))
	for _, r := range routes {
		var rest []string
		for _, h := range r.Hosts {
			g, gated := gates[h]
			if !gated {
				rest = append(rest, h)
				continue
			}
			if g == nil {
				continue
			}
			own := r
			own.Hosts = []string{h}
			own.AuthGate = g
			out = append(out, own)
		}
		if len(rest) > 0 {
			r.Hosts = rest
			out = append(out, r)
		}
	}
	return out
}

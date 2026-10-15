// Package tlsowner decides which component terminates TLS for a hostname,
// so every surface reports one coherent certificate and ACME state.
package tlsowner

import (
	"context"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Owner is the component whose certificate visitors actually receive.
type Owner string

// Owners a hostname can have.
const (
	ACME     Owner = "acme"
	Internal Owner = "internal"
	Proxy    Owner = "proxy"
	Custom   Owner = "custom"
)

// ProxyCert is the certificate a fronting proxy was last seen serving.
type ProxyCert struct {
	Issuer   string
	NotAfter time.Time
}

// Inputs are the stored facts Resolve decides from.
type Inputs struct {
	Upstream    bool
	ACMEEnabled bool
	Custom      bool
	Route       *store.ProxyRouteStatus
}

// Resolve returns the effective TLS owner. A proxy that terminates TLS wins
// over everything stored here, because the ingress then serves plain HTTP.
func Resolve(in Inputs) Owner {
	switch {
	case in.Upstream:
		return Proxy
	case in.Route != nil && in.Route.Written && in.Route.CertTrusted:
		return Proxy
	case in.Custom:
		return Custom
	case in.ACMEEnabled:
		return ACME
	default:
		return Internal
	}
}

// SettingsReader is the ingress settings read this package needs.
type SettingsReader interface {
	GetIngressSettings(ctx context.Context) (store.IngressSettings, error)
}

// RouteReader lists the managed proxy routes and what each one serves.
type RouteReader interface {
	ListProxyRouteStatus(ctx context.Context) ([]store.ProxyRouteStatus, error)
}

// Snapshot is the stored TLS ownership state at one instant.
type Snapshot struct {
	upstream bool
	acme     bool
	custom   map[string]bool
	routes   map[string]store.ProxyRouteStatus
}

// Load reads settings and route status. Either reader may be nil, and a
// read error leaves that part empty so ownership degrades to the ingress's own.
func Load(ctx context.Context, settings SettingsReader, routes RouteReader, custom map[string]bool) Snapshot {
	snap := Snapshot{custom: make(map[string]bool, len(custom)), routes: map[string]store.ProxyRouteStatus{}}
	for k, v := range custom {
		if v {
			snap.custom[strings.ToLower(k)] = true
		}
	}
	if settings != nil {
		if s, err := settings.GetIngressSettings(ctx); err == nil {
			snap.upstream, snap.acme = s.TLSTerminatedUpstream, s.EffectiveACMEEnabled()
		}
	}
	if routes != nil {
		if rows, err := routes.ListProxyRouteStatus(ctx); err == nil {
			for _, r := range rows {
				snap.routes[strings.ToLower(r.Domain)] = r
			}
		}
	}
	return snap
}

// Upstream reports whether a proxy terminates TLS for every host.
func (s Snapshot) Upstream() bool { return s.upstream }

// Owner returns the effective owner of domain.
func (s Snapshot) Owner(domain string) Owner {
	key := strings.ToLower(domain)
	in := Inputs{Upstream: s.upstream, ACMEEnabled: s.acme, Custom: s.custom[key]}
	if r, ok := s.routes[key]; ok {
		in.Route = &r
	}
	return Resolve(in)
}

// ProxyManaged reports whether a proxy owns domain's certificate.
func (s Snapshot) ProxyManaged(domain string) bool { return s.Owner(domain) == Proxy }

// Route returns the managed proxy route row for domain, if one is tracked.
func (s Snapshot) Route(domain string) (store.ProxyRouteStatus, bool) {
	r, ok := s.routes[strings.ToLower(domain)]
	return r, ok
}

// ProxyCert returns the trusted certificate the proxy serves for domain.
func (s Snapshot) ProxyCert(domain string) (ProxyCert, bool) {
	r, ok := s.routes[strings.ToLower(domain)]
	if !ok || !r.CertTrusted || r.CertNotAfter.IsZero() {
		return ProxyCert{}, false
	}
	return ProxyCert{Issuer: r.CertIssuer, NotAfter: r.CertNotAfter}, true
}

// ProxyHosts lists every tracked hostname a proxy owns.
func (s Snapshot) ProxyHosts() []string {
	var out []string
	for d := range s.routes {
		if s.ProxyManaged(d) {
			out = append(out, d)
		}
	}
	return out
}

// Suppressed reports whether a stored certificate for domain must be
// ignored: every host when TLS is upstream, else the proxy-owned ones.
func (s Snapshot) Suppressed(domain string) bool {
	return s.upstream || s.ProxyManaged(domain)
}

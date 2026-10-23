package ingress

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ProviderSource is the store surface the resolver reads; *store.DB implements it.
type ProviderSource interface {
	GetDNSProviderDefault(ctx context.Context) (store.DNSProviderChoice, error)
	ListAppDNSProviders(ctx context.Context) (map[string]store.DNSProviderChoice, error)
}

// FallbackResolver derives automatic hostnames from the instance default plus per-app overrides.
type FallbackResolver struct {
	publicHost string
	def        ProviderChoice
	overrides  map[string]ProviderChoice
}

// NewFallbackResolver builds a resolver from explicit choices.
func NewFallbackResolver(publicHost string, def ProviderChoice, overrides map[string]ProviderChoice) FallbackResolver {
	return FallbackResolver{publicHost: publicHost, def: def.Normalized(), overrides: overrides}
}

// LoadFallbackResolver reads the choices from src. A source that does not implement ProviderSource, or a read
// error, degrades to the built-in default so hostnames never disappear over a settings read.
func LoadFallbackResolver(ctx context.Context, src any, publicHost string) FallbackResolver {
	ps, ok := src.(ProviderSource)
	if !ok {
		return NewFallbackResolver(publicHost, ProviderChoice{}, nil)
	}
	var def ProviderChoice
	if d, err := ps.GetDNSProviderDefault(ctx); err == nil {
		def = ProviderChoice{Provider: d.Provider, CustomBase: d.CustomBaseDomain}
	}
	overrides := map[string]ProviderChoice{}
	if rows, err := ps.ListAppDNSProviders(ctx); err == nil {
		for app, c := range rows {
			overrides[app] = ProviderChoice{Provider: c.Provider, CustomBase: c.CustomBaseDomain}
		}
	}
	return NewFallbackResolver(publicHost, def, overrides)
}

// Default is the instance-wide choice.
func (r FallbackResolver) Default() ProviderChoice { return r.def }

// Choice is the effective choice for app and whether it is a per-app override.
func (r FallbackResolver) Choice(app string) (ProviderChoice, bool) {
	if c, ok := r.overrides[app]; ok && c.Provider != "" {
		return c.Normalized(), true
	}
	return r.def, false
}

// Domain is the automatic hostname for app under its effective provider.
func (r FallbackResolver) Domain(app string) (string, bool) {
	c, _ := r.Choice(app)
	return FallbackDomainFor(r.publicHost, app, c)
}

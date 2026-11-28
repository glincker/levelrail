// Package proxyguard decides whether an app may be the target of a static
// site proxy rule. Deploy time and serve time share it so they cannot drift.
package proxyguard

import (
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/trafficpolicy"
)

// Facts is the current access configuration of every domain.
type Facts struct {
	Domains  []store.ServiceDomain
	Gates    []store.DomainAuthGate
	Basic    []store.DomainBasicAuth
	Policies []store.DomainTrafficPolicy
}

// restrictiveKinds are the traffic controls a proxy would bypass.
var restrictiveKinds = map[string]bool{
	trafficpolicy.KindGeo:        true,
	trafficpolicy.KindShield:     true,
	trafficpolicy.KindForwarders: true,
}

// HasDomain reports whether the app is served on any domain.
func (f Facts) HasDomain(app string) bool {
	for _, d := range f.Domains {
		if d.ServiceName == app {
			return true
		}
	}
	return false
}

// Restricted is true when any domain of the app has an enabled auth gate,
// basic auth, or a geo, shield (rate limit) or forwarder policy.
func (f Facts) Restricted(app string) bool {
	mine := map[string]bool{}
	for _, d := range f.Domains {
		if d.ServiceName == app {
			mine[d.Domain] = true
		}
	}
	if len(mine) == 0 {
		return false
	}
	for _, g := range f.Gates {
		if g.Enabled && mine[g.Domain] {
			return true
		}
	}
	for _, a := range f.Basic {
		if mine[a.Domain] {
			return true
		}
	}
	for _, p := range f.Policies {
		if mine[p.Domain] && restrictiveKinds[p.Kind] {
			return true
		}
	}
	return false
}

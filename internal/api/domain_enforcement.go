package api

import (
	"context"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tlsowner"
)

// Enforcement modes reported per domain.
const (
	enforcementLevelrail     = "levelrail"
	enforcementProxyTraefik  = "proxy:traefik"
	enforcementProxyExternal = "proxy:external"
	enforcementNone          = "none"
)

// Enforcement reasons, set whenever a saved control may not be applied.
const (
	enforcementReasonNotRouted       = "not_routed"
	enforcementReasonRouteUnverified = "route_unverified"
	enforcementReasonUnmanagedProxy  = "proxy_unmanaged"
)

// domainEnforcement says, per traffic control, whether visitors to a domain
// really get it. A control saved on a domain whose flag here is false must
// never be shown as active.
type domainEnforcement struct {
	Mode        string `json:"mode"`
	Maintenance bool   `json:"maintenance"`
	BasicAuth   bool   `json:"basic_auth"`
	AuthGate    bool   `json:"auth_gate"`
	WAF         bool   `json:"waf"`
	Redirect    bool   `json:"redirect"`
	ErrorPages  bool   `json:"error_pages"`
	Reason      string `json:"reason,omitempty"`
}

func enforcementAll(mode string, on bool, reason string) domainEnforcement {
	return domainEnforcement{
		Mode: mode, Maintenance: on, BasicAuth: on, AuthGate: on, WAF: on,
		Redirect: on, ErrorPages: on, Reason: reason,
	}
}

// enforcementInput is everything resolveEnforcement decides from.
type enforcementInput struct {
	Routed bool
	// Upstream is true when a proxy terminates TLS for every host.
	Upstream bool
	Route    *store.ProxyRouteStatus
}

// resolveEnforcement fails closed: every control applies only when the
// request provably reaches this server's own ingress.
func resolveEnforcement(in enforcementInput) domainEnforcement {
	switch {
	case !in.Routed:
		return enforcementAll(enforcementNone, false, enforcementReasonNotRouted)
	case in.Route != nil && in.Route.Written:
		if proxyRouteDelivers(*in.Route) {
			return enforcementAll(enforcementProxyTraefik, true, "")
		}
		return enforcementAll(enforcementProxyTraefik, false, enforcementReasonRouteUnverified)
	case in.Upstream:
		return enforcementAll(enforcementProxyExternal, false, enforcementReasonUnmanagedProxy)
	}
	return enforcementAll(enforcementLevelrail, true, "")
}

// proxyRouteDelivers reports whether the last verification of the file as
// it is now proved the proxy hands requests to the built-in ingress.
func proxyRouteDelivers(r store.ProxyRouteStatus) bool {
	return r.Delivered && !r.VerifiedAt.IsZero() && !r.VerifiedAt.Before(r.WrittenAt)
}

// enforcementContext is loaded once per request and answers per domain.
type enforcementContext struct {
	snap tlsowner.Snapshot
}

func (rt *Router) loadEnforcement(ctx context.Context) enforcementContext {
	return enforcementContext{snap: rt.tlsSnapshot(ctx)}
}

func (ec enforcementContext) forDomain(domain string, routed bool) domainEnforcement {
	in := enforcementInput{Routed: routed, Upstream: ec.snap.Upstream()}
	if r, ok := ec.snap.Route(domain); ok {
		in.Route = &r
	}
	return resolveEnforcement(in)
}

// annotateDomainEnforcement sets each row's enforcement. A domain held
// back by an app import is not routed yet.
func (rt *Router) annotateDomainEnforcement(ctx context.Context, rows []domainResource) {
	ec := rt.loadEnforcement(ctx)
	held := rt.heldBackDomains(ctx)
	for i := range rows {
		e := ec.forDomain(rows[i].Domain, !held[strings.ToLower(rows[i].Domain)])
		rows[i].Enforcement = &e
	}
}

// appDomainEnforcement maps each of an app's domains to its enforcement;
// domains outside routed report none.
func (rt *Router) appDomainEnforcement(ctx context.Context, routed, all []string) map[string]domainEnforcement {
	ec := rt.loadEnforcement(ctx)
	isRouted := make(map[string]bool, len(routed))
	for _, d := range routed {
		isRouted[d] = true
	}
	out := make(map[string]domainEnforcement, len(all))
	for _, d := range all {
		out[d] = ec.forDomain(d, isRouted[d])
	}
	return out
}

package ingress

import (
	"context"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// TrafficPolicyStore is optional on ServiceStore: a store without it (most
// test fakes) simply routes every domain with default behavior.
type TrafficPolicyStore interface {
	ListDomainTrafficPolicies(ctx context.Context) ([]store.DomainTrafficPolicy, error)
}

const (
	targetCacheTTL     = time.Minute
	targetResolveLimit = 3 * time.Second
)

// targetCache keeps checked external forwarder addresses for a minute so a
// burst of reconcile passes does not re-resolve DNS every time, while a
// record changed to a private address is still caught within a minute.
type targetCache struct {
	mu      sync.Mutex
	entries map[string]cachedTarget
}

type cachedTarget struct {
	target  trafficpolicy.ResolvedTarget
	err     error
	expires time.Time
}

func (tc *targetCache) resolve(ctx context.Context, guard trafficpolicy.EgressGuard, resolver trafficpolicy.Resolver, rawURL string, now time.Time) (trafficpolicy.ResolvedTarget, error) {
	tc.mu.Lock()
	if e, ok := tc.entries[rawURL]; ok && now.Before(e.expires) {
		tc.mu.Unlock()
		return e.target, e.err
	}
	tc.mu.Unlock()
	rctx, cancel := context.WithTimeout(ctx, targetResolveLimit)
	defer cancel()
	t, err := guard.CheckTarget(rctx, resolver, rawURL)
	tc.mu.Lock()
	if tc.entries == nil {
		tc.entries = map[string]cachedTarget{}
	}
	tc.entries[rawURL] = cachedTarget{target: t, err: err, expires: now.Add(targetCacheTTL)}
	tc.mu.Unlock()
	return t, err
}

// applyTrafficPolicies attaches each domain's stored traffic controls to its
// proxy route. Rows that fail to decode are skipped with a warning rather
// than failing the whole pass for every other domain.
func (c *Controller) applyTrafficPolicies(ctx context.Context, routes []ingress.ProxyRoute, claimedHosts map[string]string, settings store.IngressSettings, certs []ingress.TLSCertificateOverride) ([]ingress.ProxyRoute, error) {
	ingress.SetHTTPRedirectActive(c.httpRedirect && !settings.TLSTerminatedUpstream)
	ps, ok := c.store.(TrafficPolicyStore)
	if !ok {
		return routes, nil
	}
	rows, err := ps.ListDomainTrafficPolicies(ctx)
	if err != nil {
		return routes, err
	}
	if len(rows) == 0 {
		return routes, nil
	}
	policies := map[string]*trafficpolicy.Policy{}
	for _, row := range rows {
		p := policies[row.Domain]
		if p == nil {
			p = &trafficpolicy.Policy{}
			policies[row.Domain] = p
		}
		if err := p.Decode(row.Kind, row.Spec); err != nil {
			c.logger.WarnContext(ctx, "ingress: skipping undecodable domain traffic policy",
				slog.String("domain", row.Domain), slog.String("kind", row.Kind), slog.String("error", err.Error()))
		}
	}

	appDials := map[string]string{}
	for _, r := range routes {
		if len(r.Hosts) == 0 || r.BackendDial == "" {
			continue
		}
		if owner := claimedHosts[r.Hosts[0]]; owner != "" {
			if _, seen := appDials[owner]; !seen {
				appDials[owner] = r.BackendDial
			}
		}
	}
	byoCert := map[string]bool{}
	for _, cert := range certs {
		byoCert[cert.Host] = true
	}
	limits := trafficpolicy.LimitsFromEnv(os.Getenv)
	guard := trafficpolicy.EgressGuardFromEnv(os.Getenv)
	geoActive := ingress.DefaultGeoResolver().Status().Active
	now := time.Now()

	routes = splitPolicyHosts(routes, policies)
	for i := range routes {
		if len(routes[i].Hosts) == 0 {
			continue
		}
		host := routes[i].Hosts[0]
		p, ok := policies[host]
		if !ok || p.Empty() {
			continue
		}
		rp := &ingress.RoutePolicy{
			Policy:                *p,
			AppDials:              appDials,
			Targets:               map[string]trafficpolicy.ResolvedTarget{},
			TLSTerminatedUpstream: settings.TLSTerminatedUpstream,
			TLSReal:               settings.EffectiveACMEEnabled() || byoCert[host] || settings.TLSTerminatedUpstream,
			HTTPSPort:             settings.PublicHTTPSPort,
			GeoActive:             geoActive,
			Limits:                limits,
		}
		if p.Forwarders != nil {
			for _, f := range p.Forwarders.Rules {
				if f.Action != trafficpolicy.ActionURL {
					continue
				}
				t, err := c.policyTargets.resolve(ctx, guard, net.DefaultResolver, f.URL, now)
				if err != nil {
					c.logger.WarnContext(ctx, "ingress: forwarder target refused, the rule answers 502",
						slog.String("domain", host), slog.String("url", f.URL), slog.String("error", err.Error()))
					continue
				}
				rp.Targets[f.URL] = t
			}
		}
		routes[i].Policy = rp
	}
	return routes, nil
}

// splitPolicyHosts gives every host with a policy its own route: routes
// group plain hosts together, and a policy must never spill onto a sibling.
func splitPolicyHosts(routes []ingress.ProxyRoute, policies map[string]*trafficpolicy.Policy) []ingress.ProxyRoute {
	out := make([]ingress.ProxyRoute, 0, len(routes))
	for _, r := range routes {
		if len(r.Hosts) < 2 {
			out = append(out, r)
			continue
		}
		var rest []string
		for _, h := range r.Hosts {
			if p, ok := policies[h]; ok && !p.Empty() {
				own := r
				own.Hosts = []string{h}
				out = append(out, own)
				continue
			}
			rest = append(rest, h)
		}
		if len(rest) > 0 {
			r.Hosts = rest
			out = append(out, r)
		}
	}
	return out
}

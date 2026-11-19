package ingress

import (
	"context"
	"errors"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/proxyguard"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticProxies resolves each proxy rule of a site to the internal upstream of
// the named app. The target is only ever looked up among this server's own
// services, so a rule can never reach an arbitrary address. A target that is
// missing, the site itself, or not ready gets an empty Dial and answers 502.
func (c *Controller) staticProxies(ctx context.Context, site string, rules *staticroutes.Ruleset, services []store.DesiredService, ready map[string]bool, readyAt map[string]time.Time) []ingress.StaticProxy {
	if rules == nil {
		return nil
	}
	var out []ingress.StaticProxy
	dials := map[string]string{}
	for _, r := range rules.ProxyRules() {
		app, tmpl := r.ProxyTarget()
		dial, seen := dials[app]
		if !seen {
			dial = ""
			if c.proxyScopeHolds(ctx, rules, app) && !c.proxyTargetRestricted(ctx, app, services) {
				dial = c.proxyDial(ctx, site, app, services, ready, readyAt)
			}
			dials[app] = dial
		}
		out = append(out, ingress.StaticProxy{From: r.From, Path: tmpl, Dial: dial})
	}
	return out
}

func (c *Controller) proxyDial(ctx context.Context, site, app string, services []store.DesiredService, ready map[string]bool, readyAt map[string]time.Time) string {
	if app == site {
		return ""
	}
	for _, svc := range services {
		if svc.Name != app {
			continue
		}
		if !c.isLocalNode(svc.NodeID) {
			dial, _, ok := c.dialForRemoteService(ctx, svc, ready[svc.Name])
			if !ok {
				return ""
			}
			return dial
		}
		dial, ok := c.dialForService(ctx, svc, ready[svc.Name] && c.readyForNewestAttempt(ctx, svc.Name, readyAt[svc.Name]))
		if !ok {
			return ""
		}
		return dial
	}
	return ""
}

// proxyScopeOwners is implemented by stores that know an app's team and project.
type proxyScopeOwners interface {
	OwnerOfApp(ctx context.Context, name string) (*store.ResourceOwner, error)
}

// proxyScopeHolds is false when the target now sits in another team or project
// than the one recorded at deploy or at the last rules save. It is checked on
// every pass. A ruleset with no recorded scope (written before scopes existed)
// answers 502 until the site is redeployed or its rules are saved again.
func (c *Controller) proxyScopeHolds(ctx context.Context, rules *staticroutes.Ruleset, app string) bool {
	want := rules.ProxyScopes[app]
	owners, ok := c.store.(proxyScopeOwners)
	if want == "" || !ok {
		return false
	}
	o, err := owners.OwnerOfApp(ctx, app)
	return err == nil && o != nil && o.TeamID+"/"+o.ProjectID == want
}

// proxyEnvironments is implemented by stores that can read an environment.
type proxyEnvironments interface {
	GetEnvironment(ctx context.Context, id string) (store.Environment, error)
}

// proxyTargetRestricted is true when the target now has an access gate, basic
// auth, a geo, shield or forwarder policy on any domain, or sits in a protected
// environment. It runs on every pass, so rollbacks get it too, and any lookup
// error counts as restricted.
func (c *Controller) proxyTargetRestricted(ctx context.Context, app string, services []store.DesiredService) bool {
	var f proxyguard.Facts
	for _, svc := range services {
		if svc.Name != app {
			continue
		}
		if envs, ok := c.store.(proxyEnvironments); ok && svc.EnvironmentID != "" {
			e, err := envs.GetEnvironment(ctx, svc.EnvironmentID)
			if err != nil && !errors.Is(err, store.ErrEnvironmentNotFound) {
				return true
			}
			if err == nil && e.Protected {
				return true
			}
		}
		for _, d := range svc.Domains {
			f.Domains = append(f.Domains, store.ServiceDomain{Domain: d, ServiceName: app})
		}
	}
	if len(f.Domains) == 0 {
		return false
	}
	var err error
	if f.Basic, err = c.store.ListDomainBasicAuth(ctx); err != nil {
		return true
	}
	if gs, ok := c.store.(AuthGateStore); ok {
		if f.Gates, err = gs.ListDomainAuthGates(ctx); err != nil {
			return true
		}
	}
	if ps, ok := c.store.(TrafficPolicyStore); ok {
		if f.Policies, err = ps.ListDomainTrafficPolicies(ctx); err != nil {
			return true
		}
	}
	return f.Restricted(app)
}

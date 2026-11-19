package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/proxyguard"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// proxyAppExists is the shared existence check, so a missing target and a
// forbidden one fail with the same answer.
func (rt *Router) proxyAppExists(ctx context.Context, app string) bool {
	if rt.apps == nil {
		return false
	}
	svc, err := rt.apps.GetDesiredService(ctx, app)
	return err == nil && svc != nil
}

// proxyScopeOf is the team and project of an app as one comparable key.
func (rt *Router) proxyScopeOf(ctx context.Context, app string) (string, bool, error) {
	owner, err := rt.ownerForResource(ctx, resourcePrefixApp, app)
	if err != nil {
		return "", false, err
	}
	if owner == nil {
		return "", false, nil
	}
	return owner.TeamID + "/" + owner.ProjectID, true, nil
}

// proxyFacts loads the current domain access configuration of one app.
func (rt *Router) proxyFacts(ctx context.Context, app string) (proxyguard.Facts, error) {
	var f proxyguard.Facts
	if rt.domains == nil {
		return f, nil
	}
	all, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return f, err
	}
	for _, d := range all {
		if d.ServiceName == app {
			f.Domains = append(f.Domains, d)
		}
	}
	if len(f.Domains) == 0 {
		return f, nil
	}
	if rt.domainAuthGate != nil {
		if f.Gates, err = rt.domainAuthGate.ListDomainAuthGates(ctx); err != nil {
			return f, err
		}
	}
	if rt.domainBasicAuth != nil {
		if f.Basic, err = rt.domainBasicAuth.ListDomainBasicAuth(ctx); err != nil {
			return f, err
		}
	}
	if rt.domainPolicy.store != nil {
		for _, d := range f.Domains {
			rows, err := rt.domainPolicy.store.ListDomainTrafficPoliciesFor(ctx, d.Domain)
			if err != nil {
				return f, err
			}
			f.Policies = append(f.Policies, rows...)
		}
	}
	return f, nil
}

// proxyTargetRestricted is true when the app sits behind its own access
// control (auth gate, basic auth, geo, shield or forwarders on any domain) or
// in a protected environment. A proxy would bypass that, so such targets are
// refused like missing ones. The controller applies the same rule at serve time.
func (rt *Router) proxyTargetRestricted(ctx context.Context, app string) (bool, error) {
	if _, protected, err := rt.protectedEnvironmentOf(ctx, app); err != nil || protected {
		return protected, err
	}
	f, err := rt.proxyFacts(ctx, app)
	if err != nil {
		return false, err
	}
	return f.Restricted(app), nil
}

// proxyTargetAllowed is the one target check for every interactive path:
// existence, then deploy ability, then the gate restrictions. Every refusal
// looks the same, so it is no oracle for which apps exist.
func (rt *Router) proxyTargetAllowed(ctx context.Context, c *refCaller, site, app string) (scope string, err error) {
	if app == site || !rt.proxyAppExists(ctx, app) {
		return "", deploy.ErrProxyTargetDenied
	}
	ok, err := rt.refAllowed(ctx, c, AbilityDeploy, appRef(app))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", deploy.ErrProxyTargetDenied
	}
	if restricted, err := rt.proxyTargetRestricted(ctx, app); err != nil {
		return "", err
	} else if restricted {
		return "", deploy.ErrProxyTargetDenied
	}
	scope, _, err = rt.proxyScopeOf(ctx, app)
	return scope, err
}

// proxyAuthorizerFor checks proxy targets as the interactive caller. The
// principal is captured from the request, but its policies are loaded at check
// time, so a queued or long deploy sees revoked or changed grants. The token's
// own abilities stay as they were when the request began.
func (rt *Router) proxyAuthorizerFor(r *http.Request, site string) deploy.ProxyAuthorizer {
	pt, pid, abilities, perr := rt.callerPrincipal(r)
	return func(ctx context.Context, app string) (string, error) {
		if perr != nil {
			return "", fmt.Errorf("resolve caller: %w", perr)
		}
		policies, err := rt.policies.ListPoliciesForPrincipal(ctx, pt, pid)
		if err != nil {
			return "", fmt.Errorf("list policies: %w", err)
		}
		c := &refCaller{principalType: pt, principalID: pid, abilities: abilities, policies: policies}
		return rt.proxyTargetAllowed(ctx, c, site, app)
	}
}

// liveProxyTargets are the proxy targets the site's current sidecar serves.
func (rt *Router) liveProxyTargets(ctx context.Context, site string) map[string]bool {
	out := map[string]bool{}
	g, ok := rt.staticSites.(interface {
		GetStaticSite(ctx context.Context, name string) (*store.StaticSite, error)
	})
	if !ok {
		return out
	}
	s, err := g.GetStaticSite(ctx, site)
	if err != nil || s == nil || s.RootDir == "" {
		return out
	}
	rs, err := staticroutes.LoadSidecar(s.RootDir)
	if err != nil || rs == nil {
		return out
	}
	for _, pr := range rs.ProxyRules() {
		app, _ := pr.ProxyTarget()
		if rs.ProxyScopes[app] != "" {
			out[app] = true
		}
	}
	return out
}

// proxyScopeAuthorizer is for deploys with no interactive caller (git push,
// hooks, queued replays): a target must sit in the site's own team and project,
// be unrestricted, and already be served by the live sidecar. A new target
// needs a deploy from the dashboard or CLI, where a person approves it.
func (rt *Router) proxyScopeAuthorizer(site string) deploy.ProxyAuthorizer {
	return func(ctx context.Context, app string) (string, error) {
		if app == site || !rt.proxyAppExists(ctx, app) {
			return "", deploy.ErrProxyTargetDenied
		}
		if !rt.liveProxyTargets(ctx, site)[app] {
			return "", deploy.ErrProxyTargetNew
		}
		want, ok, err := rt.proxyScopeOf(ctx, site)
		if err != nil {
			return "", err
		}
		got, found, err := rt.proxyScopeOf(ctx, app)
		if err != nil {
			return "", err
		}
		if !ok || !found || got != want {
			return "", deploy.ErrProxyTargetDenied
		}
		if restricted, err := rt.proxyTargetRestricted(ctx, app); err != nil {
			return "", err
		} else if restricted {
			return "", deploy.ErrProxyTargetDenied
		}
		return got, nil
	}
}

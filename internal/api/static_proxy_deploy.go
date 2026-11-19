package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
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

// proxyTargetRestricted is true when the app sits behind its own access
// gate (auth gate or basic auth on any domain) or in a protected environment.
// A proxy would bypass that gate, so such targets are refused like missing ones.
func (rt *Router) proxyTargetRestricted(ctx context.Context, app string) (bool, error) {
	if _, protected, err := rt.protectedEnvironmentOf(ctx, app); err != nil || protected {
		return protected, err
	}
	if rt.domains == nil {
		return false, nil
	}
	all, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return false, err
	}
	mine := map[string]bool{}
	for _, d := range all {
		if d.ServiceName == app {
			mine[d.Domain] = true
		}
	}
	if len(mine) == 0 {
		return false, nil
	}
	if rt.domainAuthGate != nil {
		gates, err := rt.domainAuthGate.ListDomainAuthGates(ctx)
		if err != nil {
			return false, err
		}
		for _, g := range gates {
			if g.Enabled && mine[g.Domain] {
				return true, nil
			}
		}
	}
	if rt.domainBasicAuth != nil {
		accts, err := rt.domainBasicAuth.ListDomainBasicAuth(ctx)
		if err != nil {
			return false, err
		}
		for _, a := range accts {
			if mine[a.Domain] {
				return true, nil
			}
		}
	}
	return false, nil
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
// caller's grants are read now, so a queued or long deploy cannot outlive the
// request's identity.
func (rt *Router) proxyAuthorizerFor(r *http.Request, site string) deploy.ProxyAuthorizer {
	c, err := rt.newRefCaller(r)
	return func(ctx context.Context, app string) (string, error) {
		if err != nil {
			return "", err
		}
		return rt.proxyTargetAllowed(ctx, c, site, app)
	}
}

// proxyScopeAuthorizer is for deploys with no interactive caller (git push,
// hooks, queued replays): a target must sit in the site's own team and project.
func (rt *Router) proxyScopeAuthorizer(site string) deploy.ProxyAuthorizer {
	return func(ctx context.Context, app string) (string, error) {
		if app == site || !rt.proxyAppExists(ctx, app) {
			return "", deploy.ErrProxyTargetDenied
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

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

// proxyAuthorizerFor checks proxy targets as the interactive caller, with the
// same read check the dashboard rules path uses. The caller's grants are read
// now, so a queued or long deploy cannot outlive the request's identity.
func (rt *Router) proxyAuthorizerFor(r *http.Request, site string) deploy.ProxyAuthorizer {
	c, err := rt.newRefCaller(r)
	return func(ctx context.Context, app string) (string, error) {
		if err != nil {
			return "", err
		}
		if app == site || !rt.proxyAppExists(ctx, app) {
			return "", deploy.ErrProxyTargetDenied
		}
		ok, aerr := rt.refAllowed(ctx, c, AbilityRead, appRef(app))
		if aerr != nil {
			return "", aerr
		}
		if !ok {
			return "", deploy.ErrProxyTargetDenied
		}
		scope, _, serr := rt.proxyScopeOf(ctx, app)
		return scope, serr
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
		return got, nil
	}
}

package ingress

import (
	"context"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticDeployRoutes serves each retained deploy of a site on its own
// immutable host, so a past deploy can be opened before it is promoted. A
// gated site's deploy hosts carry the same credential as the site itself.
func (c *Controller) staticDeployRoutes(ctx context.Context, site store.StaticSite, fallbackHost string, auth *ingress.BasicAuthAccount, claimed map[string]string) []ingress.StaticRoute {
	if fallbackHost == "" || site.RootDir == "" {
		return nil
	}
	svcDir := filepath.Dir(filepath.Clean(site.RootDir))
	if filepath.Base(svcDir) != site.Name {
		return nil
	}
	attempts, ok := c.deployAttempts(ctx, site.Name)
	if !ok {
		return nil
	}
	var routes []ingress.StaticRoute
	for _, d := range staticroutes.RetainedDeploys(attempts, svcDir) {
		host := staticroutes.DeployHost(fallbackHost, d.Attempt.ID)
		if host == "" {
			continue
		}
		if _, taken := claimed[host]; taken {
			continue
		}
		claimed[host] = site.Name
		rules, _ := staticroutes.LoadSidecar(d.Dir)
		routes = append(routes, ingress.StaticRoute{Hosts: []string{host}, RootDir: d.Dir, Rules: rules.WithoutProxy(), BasicAuth: auth})
	}
	return routes
}

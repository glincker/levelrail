package ingress

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
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
			dial = c.proxyDial(ctx, site, app, services, ready, readyAt)
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

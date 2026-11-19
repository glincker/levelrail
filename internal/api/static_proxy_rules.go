package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

// Proxy rule target states shown next to a rule.
const (
	proxyTargetOK      = "ok"
	proxyTargetMissing = "missing"
	proxyTargetSelf    = "self"
)

// proxyTargetState reports whether the app a proxy rule names can be used by
// the caller. A missing, unreadable, gated or protected app all read as missing.
func (rt *Router) proxyTargetState(ctx context.Context, c *refCaller, site, app string) string {
	if app == site {
		return proxyTargetSelf
	}
	if c == nil {
		return proxyTargetMissing
	}
	if _, err := rt.proxyTargetAllowed(ctx, c, site, app); err != nil {
		return proxyTargetMissing
	}
	return proxyTargetOK
}

// authorizeProxyRules checks every proxy target of the dashboard rules: the
// site itself is refused, and the caller must be allowed to read the app. A
// refusal and an unknown app share one message so it is no existence oracle.
func (rt *Router) authorizeProxyRules(w http.ResponseWriter, r *http.Request, site string, redirects []staticroutes.UIRedirect) bool {
	c, err := rt.newRefCaller(r)
	if err != nil {
		rt.internalError(w, "api: static site rules: proxy target authz", err)
		return false
	}
	for i, rd := range redirects {
		rule := staticroutes.Rule{To: rd.To}
		if !rule.IsProxy() {
			continue
		}
		app, _ := rule.ProxyTarget()
		msg := ""
		if app == site {
			msg = "a site cannot proxy to itself"
		} else if _, err := rt.proxyTargetAllowed(r.Context(), c, site, app); errors.Is(err, deploy.ErrProxyTargetDenied) {
			msg = "proxy target app not found"
		} else if err != nil {
			rt.internalError(w, "api: static site rules: proxy target authz", err)
			return false
		}
		if msg != "" {
			writeJSON(w, http.StatusUnprocessableEntity, staticRulesLintFailure{Error: "routing rules are invalid", Problems: []staticroutes.LineProblem{{Kind: "redirect", Index: i, Message: msg}}})
			return false
		}
	}
	return true
}

// annotateProxyRows fills the kind and target state of each rule row.
func (rt *Router) annotateProxyRows(r *http.Request, site string, rows []staticRuleRow) {
	c, _ := rt.newRefCaller(r)
	for i := range rows {
		rows[i].Kind = "route"
		rule := staticroutes.Rule{To: rows[i].To}
		if !rule.IsProxy() {
			continue
		}
		app, _ := rule.ProxyTarget()
		rows[i].Kind, rows[i].TargetApp, rows[i].TargetState = "proxy", app, rt.proxyTargetState(r.Context(), c, site, app)
	}
}

// proxyChecks reports proxy rules whose target is gone, for the site checks card.
func (rt *Router) proxyChecks(r *http.Request, site string, rs *staticroutes.Ruleset) []staticSiteCheck {
	var out []staticSiteCheck
	c, _ := rt.newRefCaller(r)
	for _, pr := range rs.ProxyRules() {
		app, _ := pr.ProxyTarget()
		if st := rt.proxyTargetState(r.Context(), c, site, app); st != proxyTargetOK {
			out = append(out, staticSiteCheck{ID: "proxy-target", Status: checkWarn, Title: "Proxy target is not reachable", Detail: pr.From + " forwards to app " + app + ", which is " + st + "; visitors get a 502", Path: pr.From})
		}
	}
	return out
}

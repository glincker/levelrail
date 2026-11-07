package ingress

import (
	"regexp"
	"strings"
)

// NextSkewRoute sends asset and data requests that carry a superseded
// Next.js deployment id to the held previous containers. Everything else
// on the host keeps going to the current release.
type NextSkewRoute struct {
	// HeldDial is the upstream of the held previous release.
	HeldDial string
	// DeploymentIDs are the ids still inside their grace window.
	DeploymentIDs []string
}

// Deployment ids come from the app's build, so only a conservative charset is
// ever put into a matcher expression.
var nextSkewIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// nextSkewPaths are the only routes an old build asks the old release for.
var nextSkewPaths = []string{"/_next/static/*", "/_next/data/*"}

// route builds the matcher route for hosts, or false when there is nothing safe to match.
func (n *NextSkewRoute) route(hosts []string, hardening *Hardening) (Route, bool) {
	if n == nil || n.HeldDial == "" {
		return Route{}, false
	}
	var clauses []string
	for _, id := range n.DeploymentIDs {
		if !nextSkewIDPattern.MatchString(id) {
			continue
		}
		clauses = append(clauses,
			"{http.request.uri.query.dpl} == '"+id+"'",
			"{http.request.header.x-deployment-id} == '"+id+"'")
	}
	if len(clauses) == 0 {
		return Route{}, false
	}
	proxy := NewReverseProxyHandler(n.HeldDial)
	if hardening != nil {
		hardening.applyProxy(&proxy)
	}
	return Route{
		Match: []Matcher{{
			Host:       hosts,
			Path:       nextSkewPaths,
			Expression: &ExpressionMatcher{Expr: strings.Join(clauses, " || "), Name: "next_skew"},
		}},
		Handle: []any{proxy},
	}, true
}

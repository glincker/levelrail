package ingress

import (
	"encoding/json"
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

// route builds the matcher route for hosts, or false when there is nothing safe
// to match. The route carries a copy of the host's full handler chain (WAF, rate
// limit, auth, maintenance, redirects) with only the upstream swapped, so an old
// build request passes the same checks as a current one.
func (n *NextSkewRoute) route(hosts []string, chain []any) (Route, bool) {
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
	handle, ok := withUpstream(chain, n.HeldDial)
	if !ok {
		return Route{}, false
	}
	return Route{
		Match: []Matcher{{
			Host:       hosts,
			Path:       nextSkewPaths,
			Expression: &ExpressionMatcher{Expr: strings.Join(clauses, " || "), Name: "next_skew"},
		}},
		Handle: handle,
	}, true
}

// withUpstream deep-copies a handler chain through JSON and points every
// app backend reverse_proxy in it at dial. The auth gate's subrequest proxy keeps its
// own address, or the forward-auth check would go to the held app. It reports false
// when the copy fails or no backend proxy was found.
func withUpstream(chain []any, dial string) ([]any, bool) {
	raw, err := json.Marshal(chain)
	if err != nil {
		return nil, false
	}
	var out []any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	swapped := false
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if t["handler"] == "reverse_proxy" && !isAuthSubrequest(t) {
				t["upstreams"] = []any{map[string]any{"dial": dial}}
				swapped = true
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(out)
	return out, swapped
}

// isAuthSubrequest spots the AuthGate check proxy: it is the only host-chain proxy that
// rewrites the request to a GET and forwards the original method in a header.
func isAuthSubrequest(proxy map[string]any) bool {
	rw, _ := proxy["rewrite"].(map[string]any)
	if rw == nil || rw["method"] == nil {
		return false
	}
	hdrs, _ := proxy["headers"].(map[string]any)
	req, _ := hdrs["request"].(map[string]any)
	set, _ := req["set"].(map[string]any)
	_, ok := set["X-Forwarded-Method"]
	return ok
}

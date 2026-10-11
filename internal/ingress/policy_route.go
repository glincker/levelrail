package ingress

import (
	"sync/atomic"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

var httpRedirectActive atomic.Bool

// SetHTTPRedirectActive records whether the running ingress redirects plain
// HTTP to HTTPS itself, so the API can explain force HTTPS behavior.
func SetHTTPRedirectActive(on bool) { httpRedirectActive.Store(on) }

// HTTPRedirectActive reports the last value the ingress reconciler set.
func HTTPRedirectActive() bool { return httpRedirectActive.Load() }

// RoutePolicy is one domain's traffic controls (internal/trafficpolicy),
// plus what the reconciler resolved for this pass: upstream dials for app
// forwarders and checked addresses for external ones.
type RoutePolicy struct {
	Policy trafficpolicy.Policy
	// AppDials maps an app name to its reverse proxy dial address.
	AppDials map[string]string
	// Targets maps an external forwarder URL to its checked address. A URL
	// missing here (blocked or unresolvable) compiles to a 502 response.
	Targets map[string]trafficpolicy.ResolvedTarget
	// TLSTerminatedUpstream means a proxy in front terminates TLS.
	TLSTerminatedUpstream bool
	// TLSReal means visitors see a publicly trusted certificate.
	TLSReal bool
	// HTTPSPort is the public HTTPS port for force HTTPS redirects.
	HTTPSPort int
	// GeoActive is false when no country source is configured; geo rules
	// are then skipped and reported as disabled.
	GeoActive bool
	Limits    trafficpolicy.Limits
}

// policyMatcher is the subset of Caddy's request matchers the policy
// handlers use; Matcher in config.go stays as other routes need it.
type policyMatcher struct {
	Path       []string           `json:"path,omitempty"`
	Method     []string           `json:"method,omitempty"`
	PathRegexp *pathRegexpMatcher `json:"path_regexp,omitempty"`
	Expression *ExpressionMatcher `json:"expression,omitempty"`
}

type pathRegexpMatcher struct {
	Name    string `json:"name,omitempty"`
	Pattern string `json:"pattern"`
}

type policyRoute struct {
	Match    []policyMatcher `json:"match,omitempty"`
	Handle   []any           `json:"handle"`
	Terminal bool            `json:"terminal,omitempty"`
}

// policySubroute runs its routes and, when none ends the request, continues
// with the next handler in the outer chain.
type policySubroute struct {
	Handler string        `json:"handler"`
	Routes  []policyRoute `json:"routes"`
}

func newPolicySubroute(routes []policyRoute) policySubroute {
	return policySubroute{Handler: "subroute", Routes: routes}
}

// wrapBackend inserts the cache and forwarder handlers directly in front of
// the route's reverse proxy (handle's last element), after auth and WAF.
func (p *RoutePolicy) wrapBackend(host string, handle []any) []any {
	if p == nil || len(handle) == 0 {
		return handle
	}
	var inserted []any
	if c := p.Policy.Cache; c != nil && c.Enabled && len(c.Rules) > 0 {
		inserted = append(inserted, newCacheHandler(host, *c, p.Limits))
	}
	if fw := p.Policy.Forwarders; fw != nil && len(fw.Rules) > 0 {
		inserted = append(inserted, p.forwardersHandler(*fw))
	}
	if len(inserted) == 0 {
		return handle
	}
	out := make([]any, 0, len(handle)+len(inserted))
	out = append(out, handle[:len(handle)-1]...)
	out = append(out, inserted...)
	return append(out, handle[len(handle)-1])
}

// wrapFront prepends redirects, geo and header handlers so they run before
// anything else on the route, WAF and basic auth included.
func (p *RoutePolicy) wrapFront(handle []any) []any {
	if p == nil {
		return handle
	}
	var front []any
	if r := p.Policy.Redirects; r != nil {
		if h, ok := newRedirectsHandler(*r, p.TLSTerminatedUpstream, p.HTTPSPort); ok {
			front = append(front, h)
		}
	}
	if g := p.Policy.Geo; g != nil && p.GeoActive {
		front = append(front, newGeoHandler(*g))
	}
	if h := p.Policy.Headers; h != nil {
		front = append(front, p.headerHandlers(*h)...)
	}
	if len(front) == 0 {
		return handle
	}
	return append(front, handle...)
}

package ingress

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// rewriteHandler is Caddy's rewrite handler (http.handlers.rewrite).
type rewriteHandler struct {
	Handler         string `json:"handler"`
	StripPathPrefix string `json:"strip_path_prefix,omitempty"`
	URI             string `json:"uri,omitempty"`
}

// forwardProxyHeaders is reverse_proxy's headers block with delete support.
type forwardProxyHeaders struct {
	Request *policyHeaderOps `json:"request,omitempty"`
}

// forwardProxyHandler is reverse_proxy for one forwarder target.
type forwardProxyHandler struct {
	Handler   string               `json:"handler"`
	Upstreams []Upstream           `json:"upstreams"`
	Transport *HTTPTransport       `json:"transport,omitempty"`
	Headers   *forwardProxyHeaders `json:"headers,omitempty"`
}

// forwardersHandler compiles ordered path rules into one subroute; the
// first matching route ends the request, and a request no rule matches
// falls through to the domain's own reverse proxy.
func (p *RoutePolicy) forwardersHandler(fw trafficpolicy.Forwarders) policySubroute {
	routes := make([]policyRoute, 0, len(fw.Rules))
	for _, f := range fw.Rules {
		routes = append(routes, policyRoute{
			Match:    []policyMatcher{forwardMatcher(f.Match)},
			Handle:   p.forwardHandle(f),
			Terminal: true,
		})
	}
	return newPolicySubroute(routes)
}

// forwardMatcher maps a trafficpolicy.Match to Caddy's path, path_regexp
// and method matchers. A prefix is segment aware: /api matches /api and
// /api/..., never /apix.
func forwardMatcher(m trafficpolicy.Match) policyMatcher {
	pm := policyMatcher{Method: m.Methods}
	switch m.Kind {
	case trafficpolicy.MatchExact:
		pm.Path = []string{m.Path}
	case trafficpolicy.MatchRegex:
		pm.PathRegexp = &pathRegexpMatcher{Pattern: m.Path}
	default:
		if prefix := strings.TrimSuffix(m.Path, "/"); prefix != "" {
			pm.Path = []string{prefix, prefix + "/*"}
		}
	}
	return pm
}

func (p *RoutePolicy) forwardHandle(f trafficpolicy.Forwarder) []any {
	if f.Action == trafficpolicy.ActionRedirect {
		status := f.RedirectStatus
		if status == 0 {
			status = http.StatusFound
		}
		return []any{NewRedirectResponseHandler(f.URL, status)}
	}
	var dial, upstreamHost, serverName string
	switch f.Action {
	case trafficpolicy.ActionApp:
		dial = p.AppDials[f.App]
		if dial == "" {
			return []any{forwardUnavailable(fmt.Sprintf("The app %s this path forwards to is not running.", f.App))}
		}
	default:
		t, ok := p.Targets[f.URL]
		if !ok {
			return []any{forwardUnavailable("The address this path forwards to is blocked by the egress policy or could not be resolved.")}
		}
		dial, serverName = t.Dial(), t.ServerName
		upstreamHost = t.Host
		if (t.Scheme == "https" && t.Port != "443") || (t.Scheme == "http" && t.Port != "80") {
			upstreamHost = t.Host + ":" + t.Port
		}
	}

	var handle []any
	if f.StripPrefix && f.Match.Kind == trafficpolicy.MatchPrefix {
		if prefix := strings.TrimSuffix(f.Match.Path, "/"); prefix != "" {
			handle = append(handle, rewriteHandler{Handler: "rewrite", StripPathPrefix: prefix})
		}
	}
	prefix := f.RewritePrefix
	if prefix == "" && f.Action == trafficpolicy.ActionURL {
		prefix = p.Targets[f.URL].BasePath
	}
	if prefix = strings.TrimSuffix(prefix, "/"); prefix != "" {
		handle = append(handle, rewriteHandler{Handler: "rewrite", URI: escapePlaceholders(prefix) + "{http.request.uri}"})
	}

	timeout := p.Limits.ForwardTimeout
	if f.TimeoutSeconds > 0 {
		timeout = time.Duration(f.TimeoutSeconds) * time.Second
	}
	rp := forwardProxyHandler{
		Handler:   "reverse_proxy",
		Upstreams: []Upstream{{Dial: dial}},
		Transport: &HTTPTransport{Protocol: "http", DialTimeout: durationString(p.Limits.ForwardDialTimeout), ResponseHeaderTimeout: durationString(timeout)},
	}
	if serverName != "" {
		rp.Transport.TLS = &UpstreamTLSConfig{ServerName: serverName}
	}
	ops := &policyHeaderOps{}
	switch f.HostMode() {
	case trafficpolicy.HostUpstream:
		if upstreamHost == "" {
			upstreamHost = "{http.reverse_proxy.upstream.hostport}"
		}
		ops.Set = map[string][]string{"Host": {upstreamHost}}
	case trafficpolicy.HostCustom:
		ops.Set = map[string][]string{"Host": {f.HostValue}}
	}
	if !f.WebSocketEnabled() {
		ops.Delete = []string{"Upgrade"}
	}
	if ops.Set != nil || ops.Delete != nil {
		rp.Headers = &forwardProxyHeaders{Request: ops}
	}
	return append(handle, rp)
}

func durationString(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

func forwardUnavailable(message string) StaticResponseHandler {
	return StaticResponseHandler{
		Handler:    "static_response",
		StatusCode: http.StatusBadGateway,
		Body:       escapePlaceholders(message) + "\n",
		Headers:    map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}},
	}
}

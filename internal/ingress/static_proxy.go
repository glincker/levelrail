package ingress

import (
	"regexp"
	"strconv"
	"strings"
)

// StaticProxy forwards one path of a static site to an app on this server.
// An empty Dial means the target is missing or down, so the path answers 502.
type StaticProxy struct {
	// From is the site path: literal segments with an optional trailing /*.
	From string
	// Path is the upstream path template; :splat is the part From's * matched.
	Path string
	Dial string
}

// proxyUnavailableBody is the only thing a visitor learns when the target is down.
const proxyUnavailableBody = "Bad gateway"

// proxySafeChars is what a captured splat may hold. Anything else (spaces, ?,
// #, non ASCII) falls through to the files, so the upstream URI cannot be
// split into a different path or query.
const proxySafeChars = `[A-Za-z0-9._~!$&'()*+,;=:@%/-]`

const proxySplatToken = ":splat"

// proxyMatchPattern is the path_regexp source for a rule's From, with the
// splat as capture group 1.
func proxyMatchPattern(from string) (pattern string, splat bool) {
	if prefix, ok := strings.CutSuffix(from, "/*"); ok {
		return `(?i)^` + regexp.QuoteMeta(prefix) + `(?:/(` + proxySafeChars + `*))?$`, true
	}
	return `(?i)^` + regexp.QuoteMeta(strings.TrimSuffix(from, "/")) + `/?$`, false
}

// proxyDotSegmentPattern matches a path under the rule holding a . or .. segment.
func proxyDotSegmentPattern(from string) string {
	prefix := strings.TrimSuffix(from, "/*")
	return `(?i)^` + regexp.QuoteMeta(prefix) + `/(?:.*/)?(?:\.|%2e){1,2}(?:/.*)?$`
}

// proxyRoutes builds, for each proxy rule, the routes that run ahead of the
// file server: a guard against dot segments and the proxy itself, both behind
// the site's password gate. Methods are all, the body streams, and Caddy sets
// X-Forwarded-* itself because no client is a trusted proxy.
func (r StaticRoute) proxyRoutes() []Route {
	var out []Route
	for i, p := range r.Proxies {
		pattern, splat := proxyMatchPattern(p.From)
		name := "lrproxy" + strconv.Itoa(i)
		match := []Matcher{{Host: r.Hosts, PathRegexp: &pathRegexpMatcher{Name: name, Pattern: pattern}}}
		if splat {
			out = append(out, Route{
				Match:  []Matcher{{Host: r.Hosts, PathRegexp: &pathRegexpMatcher{Name: name + "g", Pattern: proxyDotSegmentPattern(p.From)}}},
				Handle: []any{NewErrorPageResponse(400, "Bad request")},
			})
		}
		var handle []any
		if p.Dial == "" {
			handle = []any{NewErrorPageResponse(502, proxyUnavailableBody)}
		} else {
			rp := NewReverseProxyHandler(p.Dial)
			uri := strings.ReplaceAll(p.Path, proxySplatToken, "{http.regexp."+name+".1}") + "?{http.request.uri.query}"
			rp.Rewrite = &ProxyRewrite{URI: uri}
			rp.setRequestHeaders(map[string][]string{
				"X-Forwarded-Host":  {"{http.request.host}"},
				"X-Forwarded-Proto": {"{http.request.scheme}"},
			})
			handle = []any{rp}
		}
		if r.BasicAuth != nil {
			handle = append([]any{NewBasicAuthHandler(r.BasicAuth.Username, r.BasicAuth.Password)}, handle...)
		}
		out = append(out, Route{Match: match, Handle: handle})
	}
	return out
}

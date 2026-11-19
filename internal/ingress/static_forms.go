package ingress

import "regexp"

// FormsPathPrefix is the same-origin path a static site posts its forms to.
const FormsPathPrefix = "/_forms/"

// formsPathPattern admits one safe form name only, so encoded slashes or dot
// segments never reach the upstream path built from the capture.
const formsPathPattern = `^/_forms/([A-Za-z0-9_-]{1,64})$`

const formsMatcherName = "lrform"

var formsPathRe = regexp.MustCompile(formsPathPattern)

// formsRoute proxies POST /_forms/<name> to the control plane's public form
// endpoint, so a plain HTML form needs no CORS. It keeps the site's password gate in front.
func (r StaticRoute) formsRoute() (Route, bool) {
	if r.FormsDial == "" || r.FormsApp == "" {
		return Route{}, false
	}
	rp := NewReverseProxyHandler(r.FormsDial)
	rp.Rewrite = &ProxyRewrite{URI: "/api/v1/forms/" + r.FormsApp + "/{http.regexp.lrform.1}"}
	if len(r.FormsHeaders) > 0 {
		rp.setRequestHeaders(r.FormsHeaders)
	}
	handle := []any{rp}
	if r.BasicAuth != nil {
		handle = append([]any{NewBasicAuthHandler(r.BasicAuth.Username, r.BasicAuth.Password)}, handle...)
	}
	return Route{
		Match: []Matcher{{
			Host: r.Hosts, Method: []string{"POST", "OPTIONS"},
			PathRegexp: &pathRegexpMatcher{Name: formsMatcherName, Pattern: formsPathPattern},
		}},
		Handle: handle,
	}, true
}

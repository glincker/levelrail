package ingress

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func init() {
	caddy.RegisterModule(redirectsModule{})
}

// redirectsModule (http.handlers.levelrail_redirects) normalises a request
// before anything else runs: lower case host, force HTTPS, trailing slash.
type redirectsModule struct {
	ForceHTTPS    bool   `json:"force_https,omitempty"`
	Status        int    `json:"status,omitempty"`
	Terminated    bool   `json:"terminated,omitempty"`
	HTTPSPort     int    `json:"https_port,omitempty"`
	TrailingSlash string `json:"trailing_slash,omitempty"`
	LowercaseHost bool   `json:"lowercase_host,omitempty"`
}

// RedirectsHandler is the wire shape of redirectsModule.
type RedirectsHandler struct {
	Handler string `json:"handler"`
	redirectsModule
}

func newRedirectsHandler(r trafficpolicy.Redirects, terminated bool, httpsPort int) (RedirectsHandler, bool) {
	m := redirectsModule{
		ForceHTTPS:    r.ForceHTTPS,
		Status:        r.HTTPSStatus(),
		Terminated:    terminated,
		HTTPSPort:     httpsPort,
		TrailingSlash: r.SlashMode(),
		LowercaseHost: r.LowercaseHost,
	}
	if !m.ForceHTTPS && !m.LowercaseHost && m.TrailingSlash == trafficpolicy.SlashOff {
		return RedirectsHandler{}, false
	}
	return RedirectsHandler{Handler: "levelrail_redirects", redirectsModule: m}, true
}

func (redirectsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_redirects",
		New: func() caddy.Module { return new(redirectsModule) },
	}
}

func (m redirectsModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if loc, status, ok := m.decide(r); ok {
		w.Header().Set("Location", loc)
		w.WriteHeader(status)
		return nil
	}
	return next.ServeHTTP(w, r)
}

// decide returns the redirect for r, if any; one normalisation per hop.
func (m redirectsModule) decide(r *http.Request) (string, int, bool) {
	host := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		host = h
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if m.Terminated && m.trustedForwardedProto(r) == "https" {
		scheme = "https"
	}
	if m.LowercaseHost && host != strings.ToLower(host) {
		return scheme + "://" + strings.Replace(r.Host, host, strings.ToLower(host), 1) + r.URL.RequestURI(), http.StatusPermanentRedirect, true
	}
	if m.ForceHTTPS && m.insecure(r) {
		status := m.Status
		if status == 0 {
			status = http.StatusPermanentRedirect
		}
		target := "https://" + strings.ToLower(host)
		if m.HTTPSPort != 0 && m.HTTPSPort != 443 {
			target += ":" + strconv.Itoa(m.HTTPSPort)
		}
		return target + r.URL.RequestURI(), status, true
	}
	path := r.URL.Path
	switch m.TrailingSlash {
	case trafficpolicy.SlashAdd:
		if !strings.HasSuffix(path, "/") && !strings.Contains(path[strings.LastIndex(path, "/")+1:], ".") {
			return withPath(r, path+"/"), http.StatusPermanentRedirect, true
		}
	case trafficpolicy.SlashRemove:
		if trimmed := strings.TrimRight(path, "/"); trimmed != "" && trimmed != path {
			return withPath(r, trimmed), http.StatusPermanentRedirect, true
		}
	}
	return "", 0, false
}

// withPath keeps the request relative so the browser stays on its scheme
// and port, and keeps the query string.
func withPath(r *http.Request, path string) string {
	u := *r.URL
	u.Path = path
	u.RawPath = ""
	return u.RequestURI()
}

// insecure reports whether the visitor's connection was plain HTTP. Behind
// a TLS terminating proxy only a trusted peer's X-Forwarded-Proto counts.
func (m redirectsModule) insecure(r *http.Request) bool {
	if !m.Terminated {
		return r.TLS == nil
	}
	return m.trustedForwardedProto(r) == "http"
}

func (m redirectsModule) trustedForwardedProto(r *http.Request) string {
	trusted, _ := caddyhttp.GetVar(r.Context(), caddyhttp.TrustedProxyVarKey).(bool)
	if !trusted {
		return ""
	}
	// The trusted peer appends its own value last.
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.LastIndexByte(proto, ','); i >= 0 {
		proto = proto[i+1:]
	}
	return strings.ToLower(strings.TrimSpace(proto))
}

var (
	_ caddyhttp.MiddlewareHandler = redirectsModule{}
	_ caddy.Module                = redirectsModule{}
)

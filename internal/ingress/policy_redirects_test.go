package ingress

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestRedirectsModuleDecide(t *testing.T) {
	type req struct {
		url     string
		tls     bool
		trusted bool
		xfp     string
	}
	tests := []struct {
		name   string
		m      redirectsModule
		r      req
		loc    string
		status int
	}{
		{"plain http redirected", redirectsModule{ForceHTTPS: true, Status: 308}, req{url: "http://App.test/a?x=1"}, "https://app.test/a?x=1", 308},
		{"tls passes", redirectsModule{ForceHTTPS: true}, req{url: "https://app.test/a", tls: true}, "", 0},
		{"public port kept", redirectsModule{ForceHTTPS: true, Status: 301, HTTPSPort: 8443}, req{url: "http://app.test:8080/a"}, "https://app.test:8443/a", 301},
		{"terminated trusted http", redirectsModule{ForceHTTPS: true, Terminated: true, HTTPSPort: 443}, req{url: "http://app.test/a", trusted: true, xfp: "http"}, "https://app.test/a", 308},
		{"terminated trusted https", redirectsModule{ForceHTTPS: true, Terminated: true}, req{url: "http://app.test/a", trusted: true, xfp: "https"}, "", 0},
		{"terminated untrusted spoof ignored", redirectsModule{ForceHTTPS: true, Terminated: true}, req{url: "http://app.test/a", xfp: "http"}, "", 0},
		{"terminated last hop wins", redirectsModule{ForceHTTPS: true, Terminated: true}, req{url: "http://app.test/a", trusted: true, xfp: "https, http"}, "https://app.test/a", 308},
		{"lowercase host", redirectsModule{LowercaseHost: true}, req{url: "https://APP.test/a", tls: true}, "https://app.test/a", 308},
		{"add slash", redirectsModule{TrailingSlash: trafficpolicy.SlashAdd}, req{url: "https://app.test/docs?a=1", tls: true}, "/docs/?a=1", 308},
		{"add slash skips files", redirectsModule{TrailingSlash: trafficpolicy.SlashAdd}, req{url: "https://app.test/a.css", tls: true}, "", 0},
		{"remove slash", redirectsModule{TrailingSlash: trafficpolicy.SlashRemove}, req{url: "https://app.test/docs//", tls: true}, "/docs", 308},
		{"remove slash keeps root", redirectsModule{TrailingSlash: trafficpolicy.SlashRemove}, req{url: "https://app.test/", tls: true}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.r.url, nil)
			if tt.r.tls {
				r.TLS = &tls.ConnectionState{}
			} else {
				r.TLS = nil
			}
			if tt.r.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", tt.r.xfp)
			}
			r = r.WithContext(context.WithValue(r.Context(), caddyhttp.VarsCtxKey, map[string]any{caddyhttp.TrustedProxyVarKey: tt.r.trusted}))
			loc, status, ok := tt.m.decide(r)
			if ok != (tt.loc != "") || loc != tt.loc || status != tt.status {
				t.Fatalf("decide() = %q %d %v, want %q %d", loc, status, ok, tt.loc, tt.status)
			}
		})
	}
	if _, ok := newRedirectsHandler(trafficpolicy.Redirects{}, false, 0); ok {
		t.Error("an all-off Redirects compiled a handler")
	}
}

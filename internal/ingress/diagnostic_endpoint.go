package ingress

import (
	"crypto/subtle"
	"net/http"
)

// Headers the ingress sets on the catch-all hop to the control plane. Caddy
// overwrites both, so a client supplied value never survives the proxy.
const (
	DiagnosticTokenHeader = "X-Ingress-Diagnostic-Token" //nolint:gosec // header name, not a credential
	DiagnosticHostHeader  = "X-Ingress-Diagnostic-Host"
	DiagnosticPath        = "/internal/diagnostic"
)

// TrustedDiagnosticHandler serves inner only for requests carrying the
// per-process token the ingress adds, and takes the host from the ingress
// header. Anyone else, including a client calling the path directly, gets 404.
func TrustedDiagnosticHandler(token string, inner http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(DiagnosticTokenHeader)
		if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.Host = r.Header.Get(DiagnosticHostHeader)
		r2.Header.Del(DiagnosticTokenHeader)
		r2.Header.Del(DiagnosticHostHeader)
		inner.ServeHTTP(w, r2)
	})
}

// diagnosticProxyRoute sends the catch-all to the control plane, which
// renders the live-state page; if the hop fails the static page answers.
func diagnosticProxyRoute(h *Hardening) Route {
	rp := NewReverseProxyHandler(h.DiagnosticDial)
	rp.Rewrite = &ProxyRewrite{Method: "GET", URI: DiagnosticPath}
	rp.Headers = &ProxyHeaders{Request: &HeaderOps{Set: map[string][]string{
		DiagnosticTokenHeader: {h.DiagnosticToken},
		DiagnosticHostHeader:  {"{http.request.host}"},
	}}}
	return Route{Handle: []any{SubrouteHandler{
		Handler: "subroute",
		Routes:  []Route{{Handle: []any{rp}}},
		Errors:  &ErrorsConfig{Routes: diagnosticRoutes(404, h.BrandName, h.DocsURL)},
	}}}
}

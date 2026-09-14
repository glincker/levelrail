package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// requestIDContextKey is unexported, the standard "don't collide with
// another package's context key" convention: an exported string key
// would let any other package accidentally shadow or read this value.
type requestIDContextKey struct{}

// requestIDHeader is the response (and, when a caller already set one,
// request) header a request ID travels under: the de facto convention
// load balancers and CDNs already use, so a request arriving with one
// set (a reverse proxy in front of this control plane) is threaded
// through rather than replaced.
const requestIDHeader = "X-Request-Id"

// newRequestID mints an opaque, URL-safe correlation ID, the same
// crypto/rand-plus-base64 shape randomTokenID (tokens.go) already
// establishes for a different kind of ID.
func newRequestID() string {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failing at all is a sign of a broken host, not
		// something a request ID's own generation should ever surface
		// as a 500: fall back to a fixed, obviously-synthetic value so
		// tracing degrades to "less useful" rather than the request
		// itself failing.
		return "req_unavailable"
	}
	return "req_" + base64.RawURLEncoding.EncodeToString(buf)
}

// requestIDFromContext returns the current request's ID, or "" if
// requestIDMiddleware never ran (e.g. a unit test hitting a handler
// directly without going through Handler()).
func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// requestIDMiddleware assigns every request a correlation ID (or reuses
// one an upstream proxy already set), echoes it back on the response,
// and threads it through the request context so panicRecoveryMiddleware
// and, over time, individual handlers' own log lines can tie a symptom
// back to the exact request that caused it, without an operator having
// to timestamp-match across log lines.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// panicRecoveryMiddleware turns an unhandled panic in any handler into a
// logged, diagnosable slog entry (request ID, method, path, the panic
// value, and a stack trace) plus a clean JSON 500, instead of the
// default net/http behavior: a raw stack dump to stderr and the
// connection closed mid-response with no body at all. This is the
// single highest-leverage hardening gap on an admin control plane,
// where every panicking request is by definition an operator action
// (a deploy, a delete, a config change) that just silently failed with
// no diagnosable trace of why.
func panicRecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("api: panic recovered",
						slog.String("request_id", requestIDFromContext(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
					writeError(w, http.StatusInternalServerError, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// contentSecurityPolicy locks script execution down to the app's own
// same-origin bundle: no inline script survives web/index.html's own
// theme-init.js extraction, so script-src needs no 'unsafe-inline' or
// 'unsafe-eval'. style-src keeps 'unsafe-inline' because @xterm/xterm
// (AppTerminal.tsx's exec terminal) injects its own <style> element at
// runtime for cursor rendering; inline style is a far smaller blast
// radius than inline script (no code execution) and locking it down
// would need per-request nonce plumbing through the embedded, otherwise
// static web/dist/index.html, not worth it for the risk it removes.
// connect-src 'self' already covers the exec terminal's WebSocket and
// the log/deploy-log SSE streams: CSP3 upgrades a 'self' connect-src
// match to ws/wss automatically for a same-origin WebSocket, no explicit
// ws:／wss: entry needed.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// hstsMaxAge is 180 days: long enough to be meaningful, short enough
// that disabling APP_ENABLE_HSTS again (see Router.hstsEnabled) doesn't
// leave a browser enforcing HTTPS-only against this host indefinitely.
const hstsMaxAge = 180 * 24 * time.Hour

// securityHeadersMiddleware sets the response headers that cost nothing
// to get right and directly reduce the blast radius of an XSS or
// clickjacking attempt against a dashboard with root-level actions
// (deploys, secrets, rollbacks) behind it. Strict-Transport-Security is
// gated on hstsEnabled (Router.hstsEnabled's own doc comment explains
// why it isn't inferred automatically); everything else, including
// Content-Security-Policy, is unconditional.
func securityHeadersMiddleware(hstsEnabled bool) func(http.Handler) http.Handler {
	hsts := fmt.Sprintf("max-age=%d; includeSubDomains", int(hstsMaxAge.Seconds()))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			if hstsEnabled {
				h.Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// handleHealthz handles GET /healthz: an unauthenticated liveness check
// for systemd, a container orchestrator, or a load balancer, the one
// documented exception to "every route needs auth." Deliberately just
// "is this process alive and able to write a response," not a dependency
// check (Docker reachability, disk space): those are already covered,
// authenticated, and richer at GET /api/v1/system/doctor, which a
// human or levelrail-cli doctor calls, not a health-check probe that
// runs every few seconds.
func (rt *Router) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

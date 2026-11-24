package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	pathpkg "path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// defaultSlowRequestThreshold/defaultCriticalRequestThreshold are
// requestLoggingMiddleware's Warn/Error duration bands when
// WithRequestLogThresholds leaves a field at zero; cmd/levelrail/main.go
// resolves APP_SLOW_REQUEST_THRESHOLD/APP_CRITICAL_REQUEST_THRESHOLD and
// calls that option unconditionally, the same "this package never reads
// the environment directly" convention WithSessionTTL's own doc comment
// establishes.
const (
	defaultSlowRequestThreshold     = 500 * time.Millisecond
	defaultCriticalRequestThreshold = 2 * time.Second
)

// requestLogThresholds are requestLoggingMiddleware's Warn/Error bands.
// A zero field means "use the matching default above."
type requestLogThresholds struct {
	slow     time.Duration
	critical time.Duration
}

func (t requestLogThresholds) slowOrDefault() time.Duration {
	if t.slow > 0 {
		return t.slow
	}
	return defaultSlowRequestThreshold
}

func (t requestLogThresholds) criticalOrDefault() time.Duration {
	if t.critical > 0 {
		return t.critical
	}
	return defaultCriticalRequestThreshold
}

// requestLoggingMiddleware logs method, path, status and duration for
// every request: Debug for a normal one, Warn at the slow threshold,
// Error at the critical one, so a latency problem is findable by
// grepping a level instead of reading every line. An SSE stream or a
// hijacked WebSocket (the terminal route) stays Debug regardless of
// elapsed time, since that duration is connection lifetime, not
// server latency.
func requestLoggingMiddleware(logger *slog.Logger, thresholds requestLogThresholds) func(http.Handler) http.Handler {
	return requestLoggingMiddlewareLive(logger, thresholds, nil, nil)
}

// requestLoggingMiddlewareLive is requestLoggingMiddleware with the slow band read per request
// from slowFn (a runtime setting) and slow requests recorded into ring; either may be nil.
func requestLoggingMiddlewareLive(logger *slog.Logger, thresholds requestLogThresholds, slowFn func() time.Duration, ring *slowRequestRing) func(http.Handler) http.Handler {
	slow := thresholds.slowOrDefault()
	critical := thresholds.criticalOrDefault()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			timings := &requestTimings{}
			r = r.WithContext(context.WithValue(r.Context(), requestTimingsKey{}, timings))
			matched := &matchedPattern{}
			r = r.WithContext(context.WithValue(r.Context(), matchedPatternKey{}, matched))
			start := time.Now()
			next.ServeHTTP(rec, r)
			duration := time.Since(start)

			attrs := []slog.Attr{
				slog.String("request_id", requestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", loggablePath(r.URL.Path)),
				slog.Int("status", rec.status),
				slog.Duration("duration", duration),
			}

			curSlow, curCritical := slow, critical
			if slowFn != nil {
				if d := slowFn(); d > 0 {
					curSlow, curCritical = d, max(critical, d)
				}
			}
			streaming := isStreamingResponse(rec)
			level := requestLogLevel(duration, requestLogThresholds{slow: curSlow, critical: curCritical}, streaming)
			if level >= slog.LevelWarn {
				b := timings.breakdown()
				if b != "" {
					attrs = append(attrs, slog.String("breakdown", b))
				}
				if ring != nil {
					route := matched.get(r)
					if route == "" {
						route = loggablePath(r.URL.Path)
					}
					ring.add(slowRequestEntry{At: time.Now().UTC(), Method: r.Method, Route: route, Status: rec.status, DurationMS: float64(duration) / float64(time.Millisecond), Breakdown: b})
				}
			}
			if ring != nil && !streaming && strings.HasPrefix(r.URL.Path, "/api/") {
				ring.stats.observe(r.Method, matched.get(r), rec.status, duration)
			}
			logger.LogAttrs(r.Context(), level, "api: request", attrs...)
		})
	}
}

// requestLogLevel maps a request's duration to a log level, pulled out
// of requestLoggingMiddleware so the band logic is testable without
// depending on real elapsed time. streaming forces Debug regardless of
// duration, see requestLoggingMiddleware's own doc comment for why.
func requestLogLevel(duration time.Duration, thresholds requestLogThresholds, streaming bool) slog.Level {
	if streaming {
		return slog.LevelDebug
	}
	switch {
	case duration >= thresholds.criticalOrDefault():
		return slog.LevelError
	case duration >= thresholds.slowOrDefault():
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}

// isStreamingResponse reports whether rec served a connection that is
// long-lived by design rather than slow: an SSE response (every such
// route sets this Content-Type through startSSE) or a hijacked
// WebSocket upgrade (the terminal route, which bypasses WriteHeader
// entirely so Content-Type is never set).
func isStreamingResponse(rec *statusRecorder) bool {
	if rec.hijacked {
		return true
	}
	return strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream")
}

const imageUpdateHookPrefix = "/api/v1/hooks/image-update/"

// loggablePath hides the secret in a webhook URL (/hooks/image-update/{name}/{token}) before it reaches a log line.
func loggablePath(raw string) string {
	// Clean first so /hooks/./pages/x/secret and doubled slashes cannot dodge the prefix match.
	path := raw
	if raw != "" {
		path = pathpkg.Clean(raw)
	}
	for _, prefix := range []string{imageUpdateHookPrefix, pipelineHookPrefix, buildHookURLPrefix, pagesHookURLPrefix} {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			name, _, _ := strings.Cut(rest, "/")
			return prefix + name + "/redacted"
		}
	}
	return raw
}

type requestTimingsKey struct{}

// requestTimings collects named phase durations during one request so a
// slow request WARN line says where the time went.
type requestTimings struct {
	mu    sync.Mutex
	parts map[string]time.Duration
}

// timePhase returns a func that, when called, records the elapsed time
// under label on the request's timings. A no-op outside the middleware.
func timePhase(ctx context.Context, label string) func() {
	t, _ := ctx.Value(requestTimingsKey{}).(*requestTimings)
	if t == nil {
		return func() {}
	}
	start := time.Now()
	return func() {
		d := time.Since(start)
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.parts == nil {
			t.parts = map[string]time.Duration{}
		}
		t.parts[label] += d
	}
}

// breakdown renders phases slowest first, e.g. "telemetry_latest=1.2s,deploy_activity=40ms".
func (t *requestTimings) breakdown() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.parts) == 0 {
		return ""
	}
	labels := make([]string, 0, len(t.parts))
	for l := range t.parts {
		labels = append(labels, l)
	}
	sort.Slice(labels, func(i, j int) bool { return t.parts[labels[i]] > t.parts[labels[j]] })
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = fmt.Sprintf("%s=%s", l, t.parts[l].Round(time.Millisecond))
	}
	return strings.Join(out, ",")
}

// liveSlowThreshold is the slow band: the saved runtime setting when a service exists, else the boot option or default.
func (rt *Router) liveSlowThreshold() time.Duration {
	if rt.runtimeSettings != nil {
		if d := rt.runtimeSettings.Duration(context.Background(), runtimesettings.KeySlowRequestThreshold); d > 0 {
			return d
		}
	}
	return rt.requestLogThresholds.slowOrDefault()
}

// liveRouteStatBound is the cap on distinct routes kept for the hot routes list.
func (rt *Router) liveRouteStatBound() int {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings.Int(context.Background(), runtimesettings.KeyAPIRouteStatBound)
	}
	return routeStatBound
}

type matchedPatternKey struct{}

// matchedPattern carries the route pattern out of the inner mux: this middleware sits outside
// request copies, so r.Pattern here would be an outer catch-all mount such as "/api/".
type matchedPattern struct {
	mu sync.Mutex
	p  string
}

func (m *matchedPattern) get(r *http.Request) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.p != "" {
		return m.p
	}
	return r.Pattern
}

// capturePatternMiddleware wraps the route mux and records the pattern it matched.
func capturePatternMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if m, ok := r.Context().Value(matchedPatternKey{}).(*matchedPattern); ok && r.Pattern != "" {
			m.mu.Lock()
			m.p = r.Pattern
			m.mu.Unlock()
		}
	})
}

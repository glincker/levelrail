package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
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
	slow := thresholds.slowOrDefault()
	critical := thresholds.criticalOrDefault()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			duration := time.Since(start)

			attrs := []slog.Attr{
				slog.String("request_id", requestIDFromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", duration),
			}

			level := requestLogLevel(duration, requestLogThresholds{slow: slow, critical: critical}, isStreamingResponse(rec))
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

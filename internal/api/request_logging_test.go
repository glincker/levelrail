package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// capturingHandler is a minimal slog.Handler that records every log
// record passed to it, so a test can assert on the level and attrs
// requestLoggingMiddleware produced without parsing formatted text.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingHandler) all() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

func (h *capturingHandler) attr(name string) (slog.Value, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return slog.Value{}, false
	}
	last := h.records[len(h.records)-1]
	var v slog.Value
	found := false
	last.Attrs(func(a slog.Attr) bool {
		if a.Key == name {
			v = a.Value
			found = true
		}
		return true
	})
	return v, found
}

func TestRequestLogLevel_Bands(t *testing.T) {
	thresholds := requestLogThresholds{slow: 500 * time.Millisecond, critical: 2 * time.Second}

	cases := []struct {
		name      string
		duration  time.Duration
		streaming bool
		want      slog.Level
	}{
		{"fast", 10 * time.Millisecond, false, slog.LevelDebug},
		{"exactly slow", 500 * time.Millisecond, false, slog.LevelWarn},
		{"between slow and critical", time.Second, false, slog.LevelWarn},
		{"exactly critical", 2 * time.Second, false, slog.LevelError},
		{"well above critical", 10 * time.Second, false, slog.LevelError},
		{"streaming response ignores duration", time.Hour, true, slog.LevelDebug},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requestLogLevel(c.duration, thresholds, c.streaming); got != c.want {
				t.Errorf("requestLogLevel(%v, streaming=%v) = %v, want %v", c.duration, c.streaming, got, c.want)
			}
		})
	}
}

func TestRequestLogThresholds_DefaultsWhenZero(t *testing.T) {
	var z requestLogThresholds
	if got := z.slowOrDefault(); got != defaultSlowRequestThreshold {
		t.Errorf("slowOrDefault() = %v, want %v", got, defaultSlowRequestThreshold)
	}
	if got := z.criticalOrDefault(); got != defaultCriticalRequestThreshold {
		t.Errorf("criticalOrDefault() = %v, want %v", got, defaultCriticalRequestThreshold)
	}

	overridden := requestLogThresholds{slow: time.Second, critical: 5 * time.Second}
	if got := overridden.slowOrDefault(); got != time.Second {
		t.Errorf("slowOrDefault() = %v, want the configured 1s", got)
	}
	if got := overridden.criticalOrDefault(); got != 5*time.Second {
		t.Errorf("criticalOrDefault() = %v, want the configured 5s", got)
	}
}

func TestRequestLoggingMiddleware_CapturesStatusMethodPathAndWriteOnly(t *testing.T) {
	handler := &capturingHandler{}
	logger := slog.New(handler)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Never calls WriteHeader explicitly: statusRecorder must still
		// capture the implicit 200 on the first Write.
		_, _ = w.Write([]byte("ok"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/demo", nil)
	requestLoggingMiddleware(logger, requestLogThresholds{})(inner).ServeHTTP(rec, req)

	status, ok := handler.attr("status")
	if !ok || status.Int64() != http.StatusOK {
		t.Errorf("status attr = %v (ok=%v), want %d", status, ok, http.StatusOK)
	}
	method, ok := handler.attr("method")
	if !ok || method.String() != http.MethodGet {
		t.Errorf("method attr = %v (ok=%v), want %q", method, ok, http.MethodGet)
	}
	path, ok := handler.attr("path")
	if !ok || path.String() != "/api/v1/apps/demo" {
		t.Errorf("path attr = %v (ok=%v), want %q", path, ok, "/api/v1/apps/demo")
	}
}

func TestRequestLoggingMiddleware_ExplicitStatusCaptured(t *testing.T) {
	handler := &capturingHandler{}
	logger := slog.New(handler)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/missing", nil)
	requestLoggingMiddleware(logger, requestLogThresholds{})(inner).ServeHTTP(rec, req)

	status, ok := handler.attr("status")
	if !ok || status.Int64() != http.StatusNotFound {
		t.Errorf("status attr = %v (ok=%v), want %d", status, ok, http.StatusNotFound)
	}
	if len(handler.all()) != 1 {
		t.Fatalf("got %d log records, want 1", len(handler.all()))
	}
	if handler.all()[0].Level != slog.LevelWarn && handler.all()[0].Level != slog.LevelError && handler.all()[0].Level != slog.LevelDebug {
		t.Errorf("unexpected level %v", handler.all()[0].Level)
	}
}

func TestRequestLoggingMiddleware_SSEResponseStaysDebugRegardlessOfDuration(t *testing.T) {
	handler := &capturingHandler{}
	logger := slog.New(handler)

	// A threshold so low that any real elapsed time trips Warn/Error,
	// proving the SSE Content-Type check is what keeps this at Debug,
	// not the thresholds happening not to fire.
	tiny := requestLogThresholds{slow: time.Nanosecond, critical: 2 * time.Nanosecond}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(": connected\n\n"))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/demo/logs/stream", nil)
	requestLoggingMiddleware(logger, tiny)(inner).ServeHTTP(rec, req)

	if len(handler.all()) != 1 {
		t.Fatalf("got %d log records, want 1", len(handler.all()))
	}
	if got := handler.all()[0].Level; got != slog.LevelDebug {
		t.Errorf("level = %v, want %v for an SSE response", got, slog.LevelDebug)
	}
}

func TestRequestLoggingMiddleware_HijackedConnectionStaysDebug(t *testing.T) {
	handler := &capturingHandler{}
	logger := slog.New(handler)
	tiny := requestLogThresholds{slow: time.Nanosecond, critical: 2 * time.Nanosecond}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
			}
		}
	})

	// httptest.NewRecorder doesn't implement http.Hijacker, so this
	// test drives a real listener to get a hijackable connection.
	srv := httptest.NewServer(requestLoggingMiddleware(logger, tiny)(inner))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
	}

	if len(handler.all()) != 1 {
		t.Fatalf("got %d log records, want 1", len(handler.all()))
	}
	if got := handler.all()[0].Level; got != slog.LevelDebug {
		t.Errorf("level = %v, want %v for a hijacked connection", got, slog.LevelDebug)
	}
}

func TestIsStreamingResponse(t *testing.T) {
	plain := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if isStreamingResponse(plain) {
		t.Error("plain JSON response reported as streaming")
	}

	sse := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	sse.Header().Set("Content-Type", "text/event-stream")
	if !isStreamingResponse(sse) {
		t.Error("text/event-stream response not reported as streaming")
	}

	hijacked := &statusRecorder{ResponseWriter: httptest.NewRecorder(), hijacked: true}
	if !isStreamingResponse(hijacked) {
		t.Error("hijacked connection not reported as streaming")
	}
}

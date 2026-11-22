package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The logging middleware sits behind an outer "/api/" mount, so it must read the pattern
// the inner mux matched, not the mount.
func TestRouteStatsUseMatchedPattern(t *testing.T) {
	inner := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	inner.HandleFunc("GET /api/v1/apps/{name}", ok)
	inner.HandleFunc("GET /api/v1/databases", ok)
	inner.HandleFunc("POST /api/v1/deploys", ok)

	var ring slowRequestRing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chain := requestLoggingMiddlewareLive(logger, requestLogThresholds{}, nil, &ring)(capturePatternMiddleware(inner))
	outer := http.NewServeMux()
	outer.Handle("/api/", chain)

	for _, c := range [][2]string{{"GET", "/api/v1/apps/web"}, {"GET", "/api/v1/apps/db"}, {"GET", "/api/v1/databases"}, {"POST", "/api/v1/deploys"}} {
		outer.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c[0], c[1], nil))
	}
	top := ring.stats.top(10)
	if len(top) != 3 {
		t.Fatalf("want 3 distinct routes, got %+v", top)
	}
	for _, s := range top {
		if s.Route == "/api/" {
			t.Fatalf("route collapsed to mount: %+v", s)
		}
	}
}

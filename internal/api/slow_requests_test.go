package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlowRequestRingKeepsNewestFirstAndBounds(t *testing.T) {
	var r slowRequestRing
	for i := range slowRequestRingSize + 5 {
		r.add(slowRequestEntry{Status: i})
	}
	got := r.snapshot()
	if len(got) != slowRequestRingSize || got[0].Status != slowRequestRingSize+4 {
		t.Fatalf("len=%d first=%d, want %d entries newest first", len(got), got[0].Status, slowRequestRingSize)
	}
}

func TestLiveSlowThresholdRecordsRequest(t *testing.T) {
	var ring slowRequestRing
	live := time.Hour
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	h := requestLoggingMiddlewareLive(slog.New(slog.DiscardHandler), requestLogThresholds{}, func() time.Duration { return live }, &ring)(inner)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil))
	if n := len(ring.snapshot()); n != 0 {
		t.Fatalf("recorded %d requests under a 1h threshold, want 0", n)
	}
	live = time.Millisecond
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil))
	got := ring.snapshot()
	if len(got) != 1 || got[0].Route != "/api/v1/apps" || got[0].DurationMS < 1 {
		t.Fatalf("ring = %+v, want one entry for /api/v1/apps", got)
	}
}

func TestRouteStatsAggregateByPatternAndRank(t *testing.T) {
	var s routeStats
	s.observe("GET", "/api/v1/apps/{id}", 200, 10*time.Millisecond)
	s.observe("GET", "/api/v1/apps/{id}", 500, 30*time.Millisecond)
	s.observe("GET", "/api/v1/ping", 200, time.Millisecond)
	s.observe("GET", "", 404, time.Millisecond)
	got := s.top(2)
	if len(got) != 2 || got[0].Route != "/api/v1/apps/{id}" || got[0].Count != 2 || got[0].Errors != 1 || got[0].MaxMS < 30 || got[0].AvgMS < 19 {
		t.Fatalf("top = %+v, want the apps route first with count 2, one error, max 30, avg 20", got)
	}
	s.reset()
	if len(s.top(5)) != 0 {
		t.Fatal("reset left routes behind")
	}
}

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

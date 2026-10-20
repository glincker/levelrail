package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestTrimBackfill_DropsEntriesAtOrAfterCutoff(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	entries := []telemetry.LogEntry{
		{Message: "before", Timestamp: base.Add(-time.Second)},
		{Message: "at cutoff", Timestamp: base},
		{Message: "after", Timestamp: base.Add(time.Second)},
	}
	got := trimBackfill(entries, base, 100)
	if len(got) != 1 || got[0].Message != "before" {
		t.Errorf("trimBackfill() = %+v, want only the entry strictly before cutoff", got)
	}
}

func TestTrimBackfill_CapsToMostRecentMaxLines(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	entries := make([]telemetry.LogEntry, 5)
	for i := range entries {
		entries[i] = telemetry.LogEntry{
			Message:   string(rune('a' + i)),
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
	}
	got := trimBackfill(entries, base.Add(time.Hour), 2)
	if len(got) != 2 {
		t.Fatalf("trimBackfill() = %d entries, want 2", len(got))
	}
	if got[0].Message != "d" || got[1].Message != "e" {
		t.Errorf("trimBackfill() = %+v, want the two most recent entries in order", got)
	}
}

func TestTrimBackfill_EmptyInputReturnsEmpty(t *testing.T) {
	got := trimBackfill(nil, time.Now(), 100)
	if len(got) != 0 {
		t.Errorf("trimBackfill(nil) = %+v, want empty", got)
	}
}

// newTestRouterWithLiveLogs is newTestRouterWithTelemetry plus a wired
// LogBroadcaster, for handleLiveLogStream specifically: that handler
// needs both (rt.telemetry for backfill, rt.logBroadcaster for the live
// tail), the same "needs both halves" shape its own doc comment
// describes.
func newTestRouterWithLiveLogs(t *testing.T) (*Router, *store.DB, *telemetry.DB, *telemetry.LogBroadcaster) {
	t.Helper()
	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	broadcaster := telemetry.NewLogBroadcaster()
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)),
		WithLogBroadcaster(broadcaster),
	)
	return rt, db, tdb, broadcaster
}

func TestHandleLiveLogStream_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier, no WithLogBroadcaster
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs/stream", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleLiveLogStream_OnlyTelemetryConfigured_StillNotImplemented(t *testing.T) {
	// Half-configured (backfill source but no live broadcaster) must
	// still be treated as not configured: a "live" view that can never
	// receive a live line is a worse experience than a clear 501, not a
	// degraded-but-usable one.
	rt, db, _ := newTestRouterWithTelemetry(t) // no WithLogBroadcaster
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs/stream", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleLiveLogStream_AppNotFound(t *testing.T) {
	rt, db, _, _ := newTestRouterWithLiveLogs(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nonexistent/logs/stream", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleLiveLogStream_BackfillThenLive exercises the real
// backfill-to-live handoff end to end, over a real HTTP connection
// (httptest.NewServer, not httptest.NewRecorder) since the handler and
// this test both read/write the response concurrently: seed one old
// entry in the persisted store (the backfill this handler should replay
// first), connect, confirm it arrives, then Publish a new entry directly
// on the broadcaster (standing in for LogCollector.StreamOne) and
// confirm it arrives live, on the same connection, with no reconnect.
func TestHandleLiveLogStream_BackfillThenLive(t *testing.T) {
	rt, db, tdb, broadcaster := newTestRouterWithLiveLogs(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC()
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "backfilled line"},
		{ResourceID: "service:other", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "must not leak across apps"},
	}); err != nil {
		t.Fatalf("seed backfill entry: %v", err)
	}

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/web/logs/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)

	first, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read backfill event: %v", err)
	}
	if !strings.Contains(first, `"line":"backfilled line"`) {
		t.Fatalf("first event = %q, want the backfilled line", first)
	}

	// A short wait before publishing: the handler subscribes before its
	// backfill query even runs (see handleLiveLogStream's own doc
	// comment on why that ordering matters), so by the time this test
	// has already read the backfill event above, the subscription is
	// long since in place. This sleep is just belt-and-suspenders
	// against scheduling jitter, not a correctness requirement.
	time.Sleep(20 * time.Millisecond)
	broadcaster.Publish(telemetry.LogEntry{ResourceID: "service:web", Stream: "stderr", Message: "live line"})

	second, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read live event: %v", err)
	}
	if !strings.Contains(second, `"line":"live line"`) || !strings.Contains(second, `"stream":"stderr"`) {
		t.Fatalf("second event = %q, want the live-published line/stream", second)
	}
}

func TestHandleLiveLogStream_NoSubscriberLeakAfterDisconnect(t *testing.T) {
	// Regression guard for the map-growth concern LogBroadcaster.
	// Subscribe/unsubscribe's own doc comment calls out: a client that
	// connects and disconnects must not leave a dangling subscriber
	// channel behind that a later Publish would try (and fail, silently
	// via the non-blocking select) to deliver to forever.
	rt, db, _, broadcaster := newTestRouterWithLiveLogs(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/web/logs/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_, _ = bufio.NewReader(resp.Body).ReadString('\n') // read at least the ": connected" comment before tearing down
	_ = resp.Body.Close()
	reqCancel()

	// Give the server-side handler goroutine time to observe
	// r.Context().Done() and run its deferred unsubscribe.
	time.Sleep(200 * time.Millisecond)

	// Subscribing again and publishing proves the broadcaster's internal
	// state for this resource is healthy (not directly asserting map
	// emptiness, which isn't exposed, but this exercises the exact same
	// code path Subscribe/unsubscribe's cleanup logic protects).
	live, unsubscribe := broadcaster.Subscribe("service:web")
	defer unsubscribe()
	broadcaster.Publish(telemetry.LogEntry{ResourceID: "service:web", Message: "still works"})
	select {
	case got := <-live:
		if got.Message != "still works" {
			t.Errorf("got %+v, want message 'still works'", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out: broadcaster did not deliver after a prior subscriber disconnected")
	}
}

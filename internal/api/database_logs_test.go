package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleQueryDatabaseLogs_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/logs", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryDatabaseLogs_DatabaseNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/nonexistent/logs", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleQueryDatabaseLogs_SearchQuery(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "database:main", Stream: "stdout", Timestamp: now, Message: "database system is ready to accept connections"},
		{ResourceID: "database:main", Stream: "stdout", Timestamp: now, Message: "checkpoint complete"},
		// Must not leak across resource kinds sharing the same base name.
		{ResourceID: "service:main", Stream: "stdout", Timestamp: now, Message: "ready to accept requests"},
	})
	if err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/databases/main/logs?q=ready&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got logsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("search results = %d, want 1 (only the matching database:main line)", len(got.Entries))
	}
	if got.Entries[0].Message != "database system is ready to accept connections" {
		t.Errorf("search result = %q, want the database's own matching line", got.Entries[0].Message)
	}
}

// newTestRouterWithDatabaseLiveLogs is newTestRouterWithLiveLogs's
// database-kind counterpart: same wiring, exists only so this file
// doesn't reach into live_logs_test.go's app-scoped helper name.
func newTestRouterWithDatabaseLiveLogs(t *testing.T) (*Router, *store.DB, *telemetry.DB, *telemetry.LogBroadcaster) {
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

func TestHandleLiveDatabaseLogStream_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/logs/stream", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleLiveDatabaseLogStream_DatabaseNotFound(t *testing.T) {
	rt, db, _, _ := newTestRouterWithDatabaseLiveLogs(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/nonexistent/logs/stream", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleLiveDatabaseLogStream_BackfillThenLive(t *testing.T) {
	rt, db, tdb, broadcaster := newTestRouterWithDatabaseLiveLogs(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC()
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{
		{ResourceID: "database:main", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "backfilled db line"},
		{ResourceID: "service:main", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "must not leak across resource kinds"},
	}); err != nil {
		t.Fatalf("seed backfill entry: %v", err)
	}

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/databases/main/logs/stream", nil)
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

	reader := bufio.NewReader(resp.Body)

	first, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read backfill event: %v", err)
	}
	if !strings.Contains(first, `"line":"backfilled db line"`) {
		t.Fatalf("first event = %q, want the backfilled db line", first)
	}

	time.Sleep(20 * time.Millisecond)
	broadcaster.Publish(telemetry.LogEntry{ResourceID: "database:main", Stream: "stderr", Message: "live db line"})

	second, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read live event: %v", err)
	}
	if !strings.Contains(second, `"line":"live db line"`) || !strings.Contains(second, `"stream":"stderr"`) {
		t.Fatalf("second event = %q, want the live-published line/stream", second)
	}
}

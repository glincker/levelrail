package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleQueryDatabaseSlowQueries_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/slow-queries", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryDatabaseSlowQueries_DatabaseNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/nonexistent/slow-queries", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleQueryDatabaseSlowQueries_UnsupportedEngine(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/cache/slow-queries", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleQueryDatabaseSlowQueries_Postgres_ParsesAndSortsByDuration(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "database:main", Stream: "stderr", Timestamp: now, Message: "2026-09-20 10:00:00.000 UTC [1] LOG:  database system is ready to accept connections"},
		{ResourceID: "database:main", Stream: "stderr", Timestamp: now.Add(time.Second), Message: "2026-09-20 10:00:01.000 UTC [2] LOG:  duration: 5.500 ms  statement: SELECT 1"},
		{ResourceID: "database:main", Stream: "stderr", Timestamp: now.Add(2 * time.Second), Message: "2026-09-20 10:00:02.000 UTC [3] LOG:  duration: 1234.000 ms  statement: SELECT pg_sleep(1.234)"},
		// Must not leak across resource kinds sharing the same base name.
		{ResourceID: "service:main", Stream: "stderr", Timestamp: now, Message: "duration: 9999.000 ms  statement: SELECT 'other resource'"},
	})
	if err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/databases/main/slow-queries?from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got slowQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 {
		t.Fatalf("Total = %d, want 2", got.Total)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(got.Entries))
	}
	if got.Entries[0].DurationMs != 1234.0 || got.Entries[0].Query != "SELECT pg_sleep(1.234)" {
		t.Errorf("entry 0 = %+v, want the 1234ms pg_sleep entry sorted first", got.Entries[0])
	}
	if got.Entries[1].DurationMs != 5.5 || got.Entries[1].Query != "SELECT 1" {
		t.Errorf("entry 1 = %+v, want the 5.5ms SELECT 1 entry sorted second", got.Entries[1])
	}
}

// mysqlSlowLogFixture is what the "test -f && tail" command
// (mysqlSlowQueryEntries) gets back from a real container: MySQL's own
// slow log format, see database.MySQLSlowQueryLogPath's own doc comment
// for why this is read via exec rather than the Docker log stream.
const mysqlSlowLogFixture = "# Query_time: 2.000000  Lock_time: 0.000000 Rows_sent: 1  Rows_examined: 500000\n" +
	"SET timestamp=1704110400;\n" +
	"SELECT COUNT(*) FROM orders;\n"

func TestHandleQueryDatabaseSlowQueries_MySQL_ExecsAndParsesMultiLineBlocks(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "container-1", Running: true},
		execReader:   io.NopCloser(strings.NewReader(mysqlSlowLogFixture)),
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/slow-queries", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.execCalls != 1 {
		t.Fatalf("execCalls = %d, want 1", fake.execCalls)
	}
	if fake.gotContainerID != "container-1" {
		t.Errorf("gotContainerID = %q, want %q", fake.gotContainerID, "container-1")
	}

	var got slowQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(got.Entries))
	}
	e := got.Entries[0]
	if e.DurationMs != 2000.0 {
		t.Errorf("DurationMs = %v, want 2000", e.DurationMs)
	}
	if e.RowsExamined != 500000 {
		t.Errorf("RowsExamined = %v, want 500000", e.RowsExamined)
	}
	if e.Query != "SELECT COUNT(*) FROM orders" {
		t.Errorf("Query = %q, want %q", e.Query, "SELECT COUNT(*) FROM orders")
	}
}

// TestHandleQueryDatabaseSlowQueries_MySQL_NoLogFileYet proves an empty
// exec result (the "test -f" guard finding nothing, this database's
// slow query log hasn't been created yet because no statement has ever
// crossed the threshold) is a normal empty response, not an error.
func TestHandleQueryDatabaseSlowQueries_MySQL_NoLogFileYet(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "container-1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("")),
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/slow-queries", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got slowQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 0 || len(got.Entries) != 0 {
		t.Errorf("got %+v, want an empty response", got)
	}
}

func TestHandleQueryDatabaseSlowQueries_MySQL_ExecNotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/slow-queries", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryDatabaseSlowQueries_MySQL_NoRunningContainer(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "container-1", Running: false}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/slow-queries", ""))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleQueryDatabaseSlowQueries_LimitAndOffset(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	var entries []telemetry.LogEntry
	for i := 0; i < 5; i++ {
		entries = append(entries, telemetry.LogEntry{
			ResourceID: "database:main",
			Stream:     "stderr",
			Timestamp:  now.Add(time.Duration(i) * time.Second),
			Message:    "2026-09-20 10:00:00.000 UTC [1] LOG:  duration: " + durationFor(i) + " ms  statement: SELECT " + string(rune('0'+i)),
		})
	}
	if err := tdb.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/databases/main/slow-queries?limit=2&offset=1&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got slowQueriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 5 {
		t.Fatalf("Total = %d, want 5", got.Total)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2 (limit)", len(got.Entries))
	}
}

// durationFor returns a distinct, increasing duration string per index so
// TestHandleQueryDatabaseSlowQueries_LimitAndOffset's five entries sort
// deterministically.
func durationFor(i int) string {
	durations := []string{"100.0", "200.0", "300.0", "400.0", "500.0"}
	return durations[i]
}

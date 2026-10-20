package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleQueryLogs_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryLogs_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nonexistent/logs", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleQueryLogs_PlainAndStructured(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "plain startup line"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now, Message: `{"level":"info","msg":"ready"}`, Structured: true, FieldsJSON: `{"level":"info","msg":"ready"}`},
	})
	if err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/apps/web/logs?from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got logsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}

	plain, structured := got.Entries[0], got.Entries[1]
	if plain.Structured || plain.FieldsJSON != nil {
		t.Errorf("plain entry = %+v, want Structured=false and no FieldsJSON", plain)
	}
	if !structured.Structured || structured.FieldsJSON == nil {
		t.Errorf("structured entry = %+v, want Structured=true and non-nil FieldsJSON", structured)
	}
}

func TestHandleQueryLogs_SearchQuery(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now, Message: "connection refused to database"},
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now, Message: "server listening on port 3000"},
	})
	if err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/apps/web/logs?q=refused&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got logsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("search results = %d, want 1 (only the matching line)", len(got.Entries))
	}
	if got.Entries[0].Message != "connection refused to database" {
		t.Errorf("search result = %q, want the line containing 'refused'", got.Entries[0].Message)
	}
}

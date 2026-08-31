package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleDownloadLogs_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs/download", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDownloadLogs_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nonexistent/logs/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDownloadLogs_PlainTextAttachment(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "plain startup line"},
		{ResourceID: "service:web", Stream: "stderr", Timestamp: now, Message: "connection refused"},
	})
	if err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/apps/web/logs/download?from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
	disp := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disp, "attachment;") || !strings.Contains(disp, "app-web-logs-") {
		t.Errorf("Content-Disposition = %q, want an attachment filename mentioning app-web-logs-", disp)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "plain startup line") || !strings.Contains(body, "connection refused") {
		t.Errorf("body = %q, want both seeded lines", body)
	}
	if strings.Count(body, "\n") != 2 {
		t.Errorf("body has %d lines, want 2", strings.Count(body, "\n"))
	}
}

func TestHandleDownloadLogs_SearchQuery(t *testing.T) {
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
	url := "/api/v1/apps/web/logs/download?q=refused&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "connection refused to database") {
		t.Errorf("body missing matching line: %q", body)
	}
	if strings.Contains(body, "server listening on port 3000") {
		t.Errorf("body should not contain the non-matching line: %q", body)
	}
}

func TestHandleDownloadLogs_TrimsToMaxLines(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	entries := make([]telemetry.LogEntry, 0, logDownloadMaxLines+5)
	for i := 0; i < logDownloadMaxLines+5; i++ {
		entries = append(entries, telemetry.LogEntry{
			ResourceID: "service:web",
			Stream:     "stdout",
			Timestamp:  now.Add(time.Duration(i) * time.Millisecond),
			Message:    "line",
		})
	}
	if err := tdb.WriteLogBatch(context.Background(), entries); err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/apps/web/logs/download?from=" + now.Add(-time.Minute).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	lines := strings.Count(rec.Body.String(), "\n")
	if lines != logDownloadMaxLines {
		t.Errorf("lines = %d, want %d (capped)", lines, logDownloadMaxLines)
	}
}

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
)

func TestHandleDownloadDatabaseLogs_PlainTextAttachment(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "database:main", Stream: "stderr", Timestamp: now, Message: "checkpoint complete"},
	}); err != nil {
		t.Fatalf("seed log entries: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/databases/main/logs/download?from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "database-main-logs-") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if !strings.Contains(rec.Body.String(), "checkpoint complete") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestLogBudgetRoutes(t *testing.T) {
	tel, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "lb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	if err := tel.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: time.Now(), Message: "hello"},
	}); err != nil {
		t.Fatal(err)
	}
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithLogBudgetStore(tel))
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	const p = "/api/v1/apps/web/log-budget"
	if rec := do(http.MethodPut, p, `{"max_mb":-1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative = %d", rec.Code)
	}
	if rec := do(http.MethodPut, p, `{"max_mb":8}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"max_mb":8`) {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, p, ""); !strings.Contains(rec.Body.String(), `"max_mb":8`) {
		t.Fatalf("get = %s", rec.Body)
	}
	rec := do(http.MethodGet, "/api/v1/logs/usage", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"app":"web"`) || !strings.Contains(body, `"budget_custom":true`) {
		t.Fatalf("usage = %d %s", rec.Code, body)
	}
	if rec := do(http.MethodDelete, p, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := do(http.MethodGet, p, ""); !strings.Contains(rec.Body.String(), `"max_mb":0`) {
		t.Fatalf("get after delete = %s", rec.Body)
	}
}

func TestLogBudgetUnsetIs501(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/logs/usage", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("code = %d", rec.Code)
	}
}

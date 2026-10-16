package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestLogPipelineRoutes(t *testing.T) {
	rt, db, _ := newTraceRouter(t)
	tel, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "lp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt = NewRouter(logger, testBrand(), db, WithLogPipelineStore(tel))
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	const p = "/api/v1/apps/web/log-pipeline"

	if rec := do(http.MethodGet, p, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rules":[]`) {
		t.Fatalf("get empty = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPut, p, `{"rules":[{"kind":"redact","pattern":"(["}]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid regex = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPut, p, `{"rules":[{"kind":"drop","pattern":"healthz"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodGet, p, ""); !strings.Contains(rec.Body.String(), "healthz") {
		t.Fatalf("get after put = %s", rec.Body)
	}
	rec := do(http.MethodPost, p+"/preview", `{"rules":[{"kind":"drop","pattern":"healthz"}],"lines":["GET /healthz","hello"]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"dropped":true`) || !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodDelete, p, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
}

func TestLogPipelineUnsetIs501(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/log-pipeline", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("code = %d, want 501", rec.Code)
	}
}

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleDownloadDeployLog_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_1/logs/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDownloadDeployLog_AttemptNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_missing/logs/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDownloadDeployLog_AttemptBelongsToDifferentApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_other", ServiceName: "worker", Image: "levelrail/worker:1",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_other/logs/download", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDownloadDeployLog_FinishedAttempt_ReturnsPersistedLog(t *testing.T) {
	db := openTestDB(t)
	fakeLogStore := &fakeDeployLogQuerier{
		entries: map[string][]sseTestEntry{
			"dep_done": {
				{Message: "line one", Stream: "stdout"},
				{Message: "line two", Stream: "stderr"},
			},
		},
	}
	rt := NewRouter(nil, testBrand(), db, WithDeployLogQuerier(fakeLogStore))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_done", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_done", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_done/logs/download", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "dep_done") {
		t.Errorf("Content-Disposition = %q, want an attachment naming dep_done", cd)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "stdout line one") {
		t.Errorf("body missing line one: %q", body)
	}
	if !strings.Contains(body, "stderr line two") {
		t.Errorf("body missing line two: %q", body)
	}
}

func TestHandleDownloadDeployLog_FinishedAttempt_NoLogStoreConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDeployLogQuerier
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_done", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_done", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_done/logs/download", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDownloadDeployLog_InProgressAttempt_ReturnsBufferedLines(t *testing.T) {
	db := openTestDB(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithDeployRecorder(recorder))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_live", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	recorder.Start("dep_live")
	progress := recorder.Progress("dep_live")
	progress(build.ProgressEvent{Log: "line one", Stream: "stdout"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_live/logs/download", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "stdout line one") {
		t.Errorf("body = %q, want it to contain the buffered line", rec.Body.String())
	}

	recorder.Finish(ctx, "dep_live")
}

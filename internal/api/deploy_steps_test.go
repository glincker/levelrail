package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleDeployStepStream_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/deploys/dep_1/steps", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeployStepStream_AttemptNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/missing/steps", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeployStepStream_NoRecorderConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDeployRecorder
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_running", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_running/steps", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDeployStepStream_FinishedAttempt_Synthesizes(t *testing.T) {
	rt, db := newTestRouter(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt.deployRecorder = recorder
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	startedAt := finishedAt.Add(-time.Minute)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_done", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusSucceeded, StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_done", store.DeployAttemptStatusSucceeded, finishedAt, ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	// serveFinishedDeploySteps holds the connection open after its burst,
	// mirroring serveFinishedDeployLog (see
	// TestHandleDeployLogStream_FinishedAttempt_ReplaysPersistedLog's own
	// doc comment): a short timeout here plays the client-disconnect role
	// a real browser navigating away would.
	reqCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_done/steps", "").WithContext(reqCtx)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"step":"building"`) {
		t.Errorf("body = %q, want a synthesized building step", body)
	}
	if !strings.Contains(body, `"step":"deploying"`) || !strings.Contains(body, `"status":"done"`) {
		t.Errorf("body = %q, want a synthesized terminal deploying/done step", body)
	}
}

func TestHandleDeployStepStream_FinishedFailedAttempt_SynthesizesFailed(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.deployRecorder = deploylog.NewRecorder(nil, discardLogger())
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_failed", ServiceName: "web", Image: "levelrail/web:2",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_failed", store.DeployAttemptStatusFailed, time.Now(), "build failed"); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_failed/steps", "").WithContext(reqCtx)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"step":"failed","status":"failed"`) {
		t.Errorf("body = %q, want a synthesized failed step", rec.Body.String())
	}
}

// TestHandleDeployStepStream_LiveTail mirrors
// TestHandleDeployLogStream_LiveTail exactly, over the step stream
// instead of the log stream: a real HTTP connection is required since
// the handler and this test read/write the response concurrently.
func TestHandleDeployStepStream_LiveTail(t *testing.T) {
	db := openTestDB(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithDeployRecorder(recorder))
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
	recorder.Step("dep_live", "detecting", "running")

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/web/deploys/dep_live/steps", nil)
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
		t.Fatalf("read first event: %v", err)
	}
	if !strings.Contains(first, `"step":"detecting"`) || !strings.Contains(first, `"status":"running"`) {
		t.Fatalf("first event = %q, want the detecting/running snapshot", first)
	}

	recorder.Step("dep_live", "building", "running")

	second, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read second (live) event: %v", err)
	}
	if !strings.Contains(second, `"step":"building"`) || !strings.Contains(second, `"status":"running"`) {
		t.Fatalf("second event = %q, want building/running", second)
	}

	recorder.Finish(ctx, "dep_live")
	reqCancel()
}

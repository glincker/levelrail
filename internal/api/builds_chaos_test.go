package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

const chaosBuildBody = `{"repo_url":"https://example.com/web.git","ref":"main"}`

func TestHandleTriggerBuild_FailureModes_AttemptFailedAndLogPreserved(t *testing.T) {
	tests := []struct {
		name     string
		buildErr error
	}{
		{"docker daemon unreachable", errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")},
		{"build killed by OOM", errors.New("buildkit: process \"/bin/sh -c npm ci\" did not complete successfully: exit code: 137")},
		{"image load fails after partial output", errors.New("load image into engine: write: broken pipe")},
		{"build context cancelled", context.Canceled},
		{"deadline exceeded", context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := newFakeBuilder("", tt.buildErr)
			fetch := newFakeFetch(t.TempDir(), nil)
			db := openTestDB(t)
			telemetryDB := newTestTelemetryDB(t)
			recorder := deploylog.NewRecorder(telemetryDB, discardLogger())
			rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithDeployRecorder(recorder))
			rt.fetch = fetch.fetch
			cookie := loginTestSession(t, rt, db)
			seedWebApp(t, db)

			_, resp := postTriggerBuildAccepted(t, rt, cookie, chaosBuildBody)

			a := awaitDeployAttemptFinished(t, db, resp.ID)
			if a.Status != store.DeployAttemptStatusFailed {
				t.Errorf("Status = %q, want %q", a.Status, store.DeployAttemptStatusFailed)
			}
			if a.Error != tt.buildErr.Error() {
				t.Errorf("Error = %q, want %q", a.Error, tt.buildErr.Error())
			}

			lines, err := telemetryDB.QueryDeployLog(context.Background(), a.ID)
			if err != nil {
				t.Fatalf("QueryDeployLog() error = %v", err)
			}
			if len(lines) == 0 {
				t.Error("no persisted log lines: output emitted before the failure must survive it")
			}

			svc, err := db.GetDesiredService(context.Background(), "web")
			if err != nil {
				t.Fatalf("GetDesiredService() error = %v", err)
			}
			if svc.Image != "levelrail/web:1" {
				t.Errorf("desired image = %q, want it untouched by a failed build", svc.Image)
			}
			fetch.awaitCleanup(t)
		})
	}
}

func postTriggerBuild(t *testing.T, rt *Router, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", chaosBuildBody))
	return rec
}

func TestHandleTriggerBuild_ConcurrentSameApp_SecondRejectedNeverTwoRunning(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fb.release = make(chan struct{})
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, first := postTriggerBuildAccepted(t, rt, cookie, chaosBuildBody)
	fb.awaitCall(t)

	rec := postTriggerBuild(t, rt, cookie)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second concurrent build status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Errorf("attempts = %d, want 1: a rejected build must not leave a row behind", len(attempts))
	}

	close(fb.release)
	awaitDeployAttemptFinished(t, db, first.ID)

	_, third := postTriggerBuildAccepted(t, rt, cookie, chaosBuildBody)
	awaitDeployAttemptFinished(t, db, third.ID)
}

func TestHandleTriggerBuild_RestartRecovery_StuckRunningAttemptSweptThenBuildAccepted(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	// The row a control plane killed mid-build leaves behind.
	stuck := store.DeployAttempt{
		ID: "stuck-attempt", ServiceName: "web", Image: "web:old", Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now().Add(-time.Hour),
	}
	if err := db.SaveDeployAttempt(context.Background(), stuck); err != nil {
		t.Fatalf("SaveDeployAttempt() error = %v", err)
	}

	if rec := postTriggerBuild(t, rt, cookie); rec.Code != http.StatusConflict {
		t.Fatalf("build before startup sweep status = %d, want %d", rec.Code, http.StatusConflict)
	}

	n, err := db.FailOrphanedDeployAttempts(context.Background(), time.Now())
	if err != nil || n != 1 {
		t.Fatalf("FailOrphanedDeployAttempts() = %d, %v; want 1, nil", n, err)
	}
	got, err := db.GetDeployAttempt(context.Background(), stuck.ID)
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != store.DeployAttemptStatusFailed || got.Error == "" || got.FinishedAt == nil {
		t.Errorf("swept attempt = %+v, want failed with a reason and finish time", got)
	}

	_, resp := postTriggerBuildAccepted(t, rt, cookie, chaosBuildBody)
	if a := awaitDeployAttemptFinished(t, db, resp.ID); a.Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployAttemptStatusSucceeded)
	}
}

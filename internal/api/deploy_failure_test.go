package api

import (
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

func seedFailedDeploy(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_ok", ServiceName: "web", Image: "levelrail/web:1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: base}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_bad", ServiceName: "web", Image: "levelrail/web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusFailed, StartedAt: base.Add(time.Minute)}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_bad", store.DeployAttemptStatusFailed, base.Add(2*time.Minute), `deploy: service "web": build: failed to read dockerfile: open Dockerfile: no such file or directory`); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}
}

func TestDeployFailureSurfaces(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFailedDeploy(t, db)

	get := func(path string, out any) {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}

	var detail deployAttemptResource
	get("/api/v1/apps/web/deploys/dep_bad", &detail)
	if detail.Failure == nil || detail.Failure.Code != "dockerfile_error" || detail.Failure.DeployID != "dep_bad" || detail.Failure.App != "web" {
		t.Fatalf("detail failure = %+v", detail.Failure)
	}

	var latest deployAttemptResource
	get("/api/v1/apps/web/deploys/latest", &latest)
	if latest.ID != "dep_bad" || latest.Failure == nil {
		t.Errorf("latest = %+v", latest)
	}

	var ok deployAttemptResource
	get("/api/v1/apps/web/deploys/dep_ok", &ok)
	if ok.Failure != nil {
		t.Errorf("succeeded deploy carries a failure: %+v", ok.Failure)
	}

	var list []deployAttemptResource
	get("/api/v1/apps/web/deploy-attempts", &list)
	if len(list) != 2 || list[0].Failure == nil || list[1].Failure != nil {
		t.Errorf("attempt list failures wrong: %+v", list)
	}

	var failed []failedDeployResource
	get("/api/v1/deploys/failed", &failed)
	if len(failed) != 1 || failed[0].Failure == nil || failed[0].Failure.Code != "dockerfile_error" {
		t.Errorf("failed deploys = %+v", failed)
	}

	var deployments deploymentListResponse
	get("/api/v1/deployments", &deployments)
	var found bool
	for _, d := range deployments.Items {
		if d.ID == "dep_bad" {
			found = true
			if d.Failure == nil || d.ErrorSummary == nil {
				t.Errorf("deployments item lost failure or error_summary: %+v", d)
			}
		}
	}
	if !found {
		t.Errorf("dep_bad missing from deployments: %+v", deployments.Items)
	}

	var diag diagnosisResource
	get("/api/v1/apps/web/diagnose", &diag)
	if diag.Failure == nil || diag.Failure.Code != "dockerfile_error" {
		t.Errorf("diagnose failure = %+v", diag.Failure)
	}
}

func TestGetDeployNotFound(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFailedDeploy(t, db)
	for _, path := range []string{"/api/v1/apps/web/deploys/nope", "/api/v1/apps/ghost/deploys/latest"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestDeployFailureExcerptSkipsPreviousContainer(t *testing.T) {
	t.Parallel()
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:2", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_crash", ServiceName: "web", Image: "levelrail/web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusFailed, Reason: "ExitedDuringReadiness", StartedAt: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{
		{ResourceID: "service:web", ContainerID: "old", Stream: "stdout", Timestamp: now.Add(-30 * time.Second), Message: "starting v1 listening 3000"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordDeployExit(ctx, store.DeployExit{
		ServiceName: "web", AttemptID: "dep_crash", Image: "levelrail/web:2", Container: "web-new", ExitCode: 1,
		Reason: "ExitedDuringReadiness", StartedAt: now.Add(-20 * time.Second), FinishedAt: now.Add(-19 * time.Second),
		LogTail: []store.DeployExitLine{{Timestamp: now, Stream: "stderr", Message: "fatal: DATABASE_URL is required"}},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_crash", ""))
	var got deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Failure == nil || !strings.Contains(got.Failure.LogExcerpt, "DATABASE_URL") || strings.Contains(got.Failure.LogExcerpt, "starting v1") {
		t.Fatalf("failure = %+v, want an excerpt from the failing container only", got.Failure)
	}
}

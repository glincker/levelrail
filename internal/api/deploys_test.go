package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleTriggerDeploy(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"levelrail/web:2"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:2" {
		t.Errorf("Image = %q, want %q", svc.Image, "levelrail/web:2")
	}
	// Fields outside Image must survive the deploy untouched.
	if svc.Port != 3000 {
		t.Errorf("Port = %d, want unchanged 3000", svc.Port)
	}

	// Deploying an app that doesn't exist.
	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/deploys", `{"image":"x:1"}`))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recMissing.Code, http.StatusNotFound)
	}

	// Missing image field on an app that does exist: half-succeeded
	// input must not touch the store.
	recNoImage := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recNoImage, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{}`))
	if recNoImage.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recNoImage.Code, http.StatusBadRequest)
	}
	svcAfter, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svcAfter.Image != "levelrail/web:2" {
		t.Errorf("a rejected deploy request must not change desired state, Image = %q", svcAfter.Image)
	}
}

// TestHandleTriggerDeploy_RecordsDeployAttempt covers this task's own
// judgment call for the plain image-tag path: it has no real build step
// (no Dockerfile build happens, see handleTriggerDeploy's own doc
// comment), but still deserves a deploy_attempts row for history and
// rollback purposes, the same way the two build-triggering paths do.
// Unlike those two, there is no build output to persist, so the attempt
// is saved and immediately finished as succeeded in the same call,
// rather than started as "running" and finished later by a background
// build.
func TestHandleTriggerDeploy_RecordsDeployAttempt(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"levelrail/web:2"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	attempts, err := db.ListDeployAttempts(ctx, "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 deploy attempt recorded, got %d", len(attempts))
	}
	a := attempts[0]
	if a.Image != "levelrail/web:2" {
		t.Errorf("Image = %q, want %q", a.Image, "levelrail/web:2")
	}
	if a.Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployAttemptStatusSucceeded)
	}
	if a.CommitSHA != "" {
		t.Errorf("CommitSHA = %q, want empty: a bare image tag isn't tied to a commit", a.CommitSHA)
	}
	if a.Source != store.DeployAttemptSourceImage {
		t.Errorf("Source = %q, want %q", a.Source, store.DeployAttemptSourceImage)
	}
	if a.FinishedAt == nil {
		t.Error("FinishedAt = nil, want set: the plain image-tag path has no build step, so the attempt finishes in the same call it starts")
	}

	// A second trigger must add a second attempt row, not overwrite the
	// first: real history, not a single "last deploy" pointer.
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"levelrail/web:3"}`))
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusAccepted)
	}
	attemptsAfter, err := db.ListDeployAttempts(ctx, "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attemptsAfter) != 2 {
		t.Fatalf("expected 2 deploy attempts after a second trigger, got %d", len(attemptsAfter))
	}
}

func TestHandleDeployHistory(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	// No reconcile has run yet: empty, not an error.
	recEmpty := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recEmpty, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys", ""))
	if recEmpty.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recEmpty.Code, http.StatusOK)
	}
	var empty []reconcile.Condition
	if err := json.Unmarshal(recEmpty.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no conditions yet, got %d", len(empty))
	}

	if err := db.UpsertConditions(ctx, applicationControllerName("web"), []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Created"},
	}); err != nil {
		t.Fatalf("seed conditions: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []reconcile.Condition
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Reason != "Created" {
		t.Fatalf("got %+v, want one condition with Reason=Created", got)
	}

	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/deploys", ""))
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recMissing.Code, http.StatusNotFound)
	}
}

func TestApplicationControllerName(t *testing.T) {
	if got, want := applicationControllerName("web"), "application/web"; got != want {
		t.Errorf("applicationControllerName(%q) = %q, want %q", "web", got, want)
	}
}

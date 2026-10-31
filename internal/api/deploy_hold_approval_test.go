package api

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func holdTestDeploy(t *testing.T, rt *Router, db *store.DB) (approvalID string, attemptID string) {
	t.Helper()
	env, err := db.GetEnvironment(context.Background(), "env_prod")
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	id, err := rt.HoldForApproval(context.Background(), env, "web", "levelrail/web:2", store.DeployAttemptSourceWebhook,
		deploy.HeldRequest{Kind: deploy.HeldKindImage, Image: "levelrail/web:2"}, SystemApprovalActor)
	if err != nil {
		t.Fatalf("HoldForApproval() error = %v", err)
	}
	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("load approval: %v", err)
	}
	return id, a.HeldAttemptID
}

func newHoldRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db, _, approver := newProtectedSafetyRouter(t, nil)
	return rt, db, approver
}

func TestAwaitingApprovalAttemptIsNotReleasedByFreezeReleaser(t *testing.T) {
	rt, db, _ := newHoldRouter(t)
	_, attemptID := holdTestDeploy(t, rt, db)
	called := false
	rel := deploy.Releaser{
		Store: db, Freeze: db, Now: time.Now, Logger: slog.Default(),
		Handlers: map[string]deploy.ReleaseHandler{deploy.HeldKindImage: func(context.Context, store.DeployAttempt, deploy.HeldRequest) error {
			called = true
			return nil
		}},
	}
	n, err := rel.ReleaseDue(context.Background())
	if err != nil || n != 0 || called {
		t.Fatalf("ReleaseDue() = %d, %v, handler called %v; want nothing replayed", n, err, called)
	}
	a, err := db.GetDeployAttempt(context.Background(), attemptID)
	if err != nil || a.Status != store.DeployAttemptStatusAwaitingApproval {
		t.Fatalf("attempt = %+v, %v; want awaiting_approval", a, err)
	}
}

func TestApprovingHeldDeployReplaysOnce(t *testing.T) {
	rt, db, approver := newHoldRouter(t)
	id, attemptID := holdTestDeploy(t, rt, db)
	if rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", "")); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	a, err := db.GetDeployAttempt(context.Background(), attemptID)
	if err != nil || a.Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("attempt = %+v, %v; want succeeded", a, err)
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil || svc.Image != "levelrail/web:2" {
		t.Fatalf("desired = %+v, %v; want replayed image", svc, err)
	}
	if rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", "")); rec.Code != http.StatusConflict {
		t.Fatalf("second approve = %d, want conflict", rec.Code)
	}
	all, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	replayed := 0
	for _, x := range all {
		if x.Image == "levelrail/web:2" && x.ID != attemptID {
			replayed++
		}
	}
	if replayed > 1 {
		t.Fatalf("replay produced %d extra attempts, want at most 1", replayed)
	}
}

func TestRejectingHeldDeployFailsAttemptWithReason(t *testing.T) {
	rt, db, approver := newHoldRouter(t)
	id, attemptID := holdTestDeploy(t, rt, db)
	if rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/reject", `{"reason":"not now"}`)); rec.Code != http.StatusOK {
		t.Fatalf("reject = %d %s", rec.Code, rec.Body.String())
	}
	a, err := db.GetDeployAttempt(context.Background(), attemptID)
	if err != nil || a.Status != store.DeployAttemptStatusFailed || a.Reason != "Rejected: not now" {
		t.Fatalf("attempt = %+v, %v; want failed with reason", a, err)
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "levelrail/web:1" {
		t.Fatalf("desired image = %q, want unchanged", svc.Image)
	}
}

package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newProtectedSafetyRouter(t *testing.T, resolver docker.ImageResolver) (*Router, *store.DB, *http.Cookie, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithDeploySafety(db, resolver))
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	approver := storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	return rt, db, requester, sessionCookieForTest(t, rt, approver.ID)
}

func pendingApprovalID(t *testing.T, rt *Router, cookie *http.Cookie, body string) string {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("request approval: %d %s", rec.Code, rec.Body.String())
	}
	approvals, err := rt.deployApprovals.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if err != nil || len(approvals) != 1 {
		t.Fatalf("pending approvals = %+v, %v", approvals, err)
	}
	return approvals[0].ID
}

func TestApproveDeployRespectsFreezeStartedAfterRequest(t *testing.T) {
	rt, db, requester, approver := newProtectedSafetyRouter(t, nil)
	id := pendingApprovalID(t, rt, requester, `{"image":"levelrail/web:2","confirm":true}`)
	if put := serve(rt, authedRequest(t, requester, http.MethodPut, "/api/v1/apps/web/deploy-freeze", alwaysFrozen())); put.Code != http.StatusOK {
		t.Fatalf("put freeze = %d", put.Code)
	}
	rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code != http.StatusLocked {
		t.Fatalf("approve during freeze = %d %s, want 423", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "levelrail/web:1" {
		t.Fatalf("image changed during freeze: %q", svc.Image)
	}
	a, _ := db.GetDeployApproval(context.Background(), id)
	if a.Status != store.DeployApprovalStatusPending {
		t.Fatalf("approval status = %q, want still pending", a.Status)
	}
}

func TestApproveDeployHonorsRequestFreezeOverride(t *testing.T) {
	rt, db, requester, approver := newProtectedSafetyRouter(t, nil)
	if put := serve(rt, authedRequest(t, requester, http.MethodPut, "/api/v1/apps/web/deploy-freeze", alwaysFrozen())); put.Code != http.StatusOK {
		t.Fatalf("put freeze = %d", put.Code)
	}
	id := pendingApprovalID(t, rt, requester, `{"image":"levelrail/web:2","confirm":true,"override_freeze":true,"override_reason":"hotfix"}`)
	rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "web")
	if len(attempts) != 1 || attempts[0].Reason != "FreezeOverride: hotfix" {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestApprovePromoteAppliesRequestedEnv(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	ctx := context.Background()
	if err := db.SetEnvironmentProtected(ctx, "env_prod", true); err != nil {
		t.Fatal(err)
	}
	src, _ := db.GetDesiredService(ctx, "web-staging")
	src.Env = map[string]string{"FEATURE_X": "on"}
	if err := db.SaveDesiredService(ctx, *src); err != nil {
		t.Fatal(err)
	}
	rec := serve(rt, authedRequest(t, requester, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","confirm":true,"include_env":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	approvals, _ := db.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, "web-prod")
	if len(approvals) != 1 || !approvals[0].IncludeEnv {
		t.Fatalf("approvals = %+v", approvals)
	}
	approver := storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	ok := serve(rt, authedRequest(t, sessionCookieForTest(t, rt, approver.ID), http.MethodPost, "/api/v1/deploy-approvals/"+approvals[0].ID+"/approve", ""))
	if ok.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", ok.Code, ok.Body.String())
	}
	target, _ := db.GetDesiredService(ctx, "web-prod")
	if target.Env["FEATURE_X"] != "on" || target.Image != "levelrail/web:2" {
		t.Fatalf("target = image %q env %v", target.Image, target.Env)
	}
}

func TestApproveDeployKeepsPullRequirement(t *testing.T) {
	rt, db, requester, approver := newProtectedSafetyRouter(t, stubResolver{err: errors.New("registry down")})
	id := pendingApprovalID(t, rt, requester, `{"image":"levelrail/web:2","confirm":true,"pull":true}`)
	if a, _ := db.GetDeployApproval(context.Background(), id); !a.Pull {
		t.Fatalf("approval lost pull: %+v", a)
	}
	rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code == http.StatusOK {
		t.Fatalf("approve with pull and registry down succeeded: %s", rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "levelrail/web:1" {
		t.Fatalf("image changed without a fresh pull: %q", svc.Image)
	}
}

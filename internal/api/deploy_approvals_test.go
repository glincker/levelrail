package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// seedProtectedDeployFixture seeds a project, a protected environment, and
// an app tagged with it, the shared setup every deploy-approval test
// below needs before it can trigger a deploy that requires approval.
func seedProtectedDeployFixture(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", Protected: true, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_prod"); err != nil {
		t.Fatalf("tag app: %v", err)
	}
}

// requestPendingApprovalImage is the image every deploy_approvals_test.go
// scenario requests: the exact value doesn't matter to any of them, only
// that it's consistent between the request and each test's own
// assertions.
const requestPendingApprovalImage = "levelrail/web:2"

// requestPendingApproval drives POST /api/v1/apps/web/deploys with
// confirm: true as requesterCookie, asserts it comes back as a pending
// approval, and returns the minted approval ID.
func requestPendingApproval(t *testing.T, rt *Router, requesterCookie *http.Cookie) string {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, requesterCookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"`+requestPendingApprovalImage+`","confirm":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("request approval: status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var result deployTriggerResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.PendingApproval == nil {
		t.Fatalf("expected a pending approval, got body %s", rec.Body.String())
	}
	return result.PendingApproval.ID
}

func TestHandleApproveDeployApproval(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)

	approver := storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	approverCookie := sessionCookieForTest(t, rt, approver.ID)

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, approverCookie, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var decision deployApprovalDecisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decision.Approval.Status != store.DeployApprovalStatusApproved {
		t.Errorf("Approval.Status = %q, want %q", decision.Approval.Status, store.DeployApprovalStatusApproved)
	}
	if decision.Approval.ApprovedBy != approver.ID {
		t.Errorf("Approval.ApprovedBy = %q, want %q", decision.Approval.ApprovedBy, approver.ID)
	}
	if decision.App.Image != "levelrail/web:2" {
		t.Errorf("App.Image = %q, want %q", decision.App.Image, "levelrail/web:2")
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:2" {
		t.Errorf("an approved deploy must actually update desired state, Image = %q", svc.Image)
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1 (an approved deploy must go through the normal deploy-attempt recording path)", len(attempts))
	}

	// Approving an already-decided approval must fail, not re-apply.
	recAgain := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recAgain, authedRequest(t, approverCookie, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if recAgain.Code != http.StatusConflict {
		t.Fatalf("re-approve: status = %d, want %d", recAgain.Code, http.StatusConflict)
	}
}

// TestHandleApproveDeployApproval_SameActorForbidden is this feature's
// central RBAC guarantee: the same user who requested a deploy into a
// protected environment cannot also be the one who approves it, even
// though that user holds AbilityDeploy (they were able to request the
// deploy in the first place, using that same ability).
func TestHandleApproveDeployApproval_SameActorForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, requester, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("self-approve: status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:1" {
		t.Errorf("a rejected self-approval must not change desired state, Image = %q", svc.Image)
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusPending {
		t.Errorf("Status = %q, want still pending after a rejected self-approval attempt", a.Status)
	}
}

func TestHandleRejectDeployApproval(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)

	approver := storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	approverCookie := sessionCookieForTest(t, rt, approver.ID)

	id := requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, approverCookie, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/reject", `{"reason":"not tonight"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("reject: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var a deployApprovalResource
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if a.Status != store.DeployApprovalStatusRejected {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployApprovalStatusRejected)
	}
	if a.Reason != "not tonight" {
		t.Errorf("Reason = %q, want %q", a.Reason, "not tonight")
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:1" {
		t.Errorf("a rejected deploy approval must never proceed, Image = %q", svc.Image)
	}
}

// TestHandleApproveDeployApproval_Expired covers the timeout path: a
// pending approval past its TTL is treated as expired and can no longer
// be approved, even by a legitimate, distinct approver.
func TestHandleApproveDeployApproval_Expired(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	rt.deployApprovalTTL = time.Millisecond

	approver := storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	approverCookie := sessionCookieForTest(t, rt, approver.ID)

	id := requestPendingApproval(t, rt, requester)
	time.Sleep(5 * time.Millisecond)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, approverCookie, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("approve expired: status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDeployApproval: %v", err)
	}
	if a.Status != store.DeployApprovalStatusExpired {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployApprovalStatusExpired)
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "levelrail/web:1" {
		t.Errorf("an expired approval must never proceed, Image = %q", svc.Image)
	}
}

func TestHandleListDeployApprovals(t *testing.T) {
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)

	requestPendingApproval(t, rt, requester)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, requester, http.MethodGet, "/api/v1/deploy-approvals", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listResp deployApprovalListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(listResp.Approvals) != 1 {
		t.Fatalf("len(Approvals) = %d, want 1", len(listResp.Approvals))
	}
	if listResp.Approvals[0].ServiceName != "web" {
		t.Errorf("ServiceName = %q, want %q", listResp.Approvals[0].ServiceName, "web")
	}
}

func TestDeployApprovalRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/deploy-approvals"},
		{http.MethodGet, "/api/v1/deploy-approvals/apr_x"},
		{http.MethodPost, "/api/v1/deploy-approvals/apr_x/approve"},
		{http.MethodPost, "/api/v1/deploy-approvals/apr_x/reject"},
	})
}

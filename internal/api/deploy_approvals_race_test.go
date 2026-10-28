package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// pausedApprove starts an approve that stops right before its claim and
// returns a release func that lets it finish and reports its status.
func pausedApprove(t *testing.T, rt *Router, cookie *http.Cookie, id string) func() int {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	approvalBeforeClaim = func() {
		approvalBeforeClaim = nil
		close(entered)
		<-release
	}
	t.Cleanup(func() { approvalBeforeClaim = nil })
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/approve", ""))
		done <- rec.Code
	}()
	<-entered
	return func() int {
		close(release)
		return <-done
	}
}

func raceFixture(t *testing.T) (*Router, *store.DB, *http.Cookie, *http.Cookie, string) {
	t.Helper()
	rt, db := newTestRouter(t)
	requester := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	b := storeUserWithAbilitiesForTest(t, db, "b@example.com", []string{AbilityRead, AbilityDeploy})
	c := storeUserWithAbilitiesForTest(t, db, "c@example.com", []string{AbilityRead, AbilityDeploy})
	return rt, db, sessionCookieForTest(t, rt, b.ID), sessionCookieForTest(t, rt, c.ID), requestPendingApproval(t, rt, requester)
}

func postApproval(t *testing.T, rt *Router, cookie *http.Cookie, id, verb string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/deploy-approvals/"+id+"/"+verb, ""))
	return rec.Code
}

func TestDeployApprovalRace_RejectWinsOverInFlightApprove(t *testing.T) {
	rt, db, b, c, id := raceFixture(t)
	finish := pausedApprove(t, rt, c, id)
	if code := postApproval(t, rt, b, id, "reject"); code != http.StatusOK {
		t.Fatalf("reject = %d", code)
	}
	if code := finish(); code != http.StatusConflict {
		t.Fatalf("approve after reject = %d, want 409", code)
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	a, _ := db.GetDeployApproval(context.Background(), id)
	if a.Status != store.DeployApprovalStatusRejected || svc.Image != "levelrail/web:1" {
		t.Fatalf("status %q image %q: a rejected approval must not deploy", a.Status, svc.Image)
	}
}

func TestDeployApprovalRace_TwoApprovesApplyOnce(t *testing.T) {
	rt, db, b, c, id := raceFixture(t)
	finish := pausedApprove(t, rt, c, id)
	if code := postApproval(t, rt, b, id, "approve"); code != http.StatusOK {
		t.Fatalf("first approve = %d", code)
	}
	if code := finish(); code != http.StatusConflict {
		t.Fatalf("second approve = %d, want 409", code)
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "web")
	if len(attempts) != 1 {
		t.Fatalf("deploy attempts = %d, want 1", len(attempts))
	}
}

func TestDeployApprovalRace_ConcurrentApproveRejectOneOutcome(t *testing.T) {
	rt, db, b, c, id := raceFixture(t)
	codes := make(chan int, 2)
	go func() { codes <- postApproval(t, rt, c, id, "approve") }()
	go func() { codes <- postApproval(t, rt, b, id, "reject") }()
	ok := 0
	for range 2 {
		if <-codes == http.StatusOK {
			ok++
		}
	}
	a, _ := db.GetDeployApproval(context.Background(), id)
	svc, _ := db.GetDesiredService(context.Background(), "web")
	applied := svc.Image == requestPendingApprovalImage
	if ok != 1 || applied != (a.Status == store.DeployApprovalStatusApproved) {
		t.Fatalf("ok=%d status=%q applied=%v", ok, a.Status, applied)
	}
}

func TestDeployApprovalRace_FailedApplyReleasesClaim(t *testing.T) {
	rt, db, b, _, id := raceFixture(t)
	if err := db.DeleteDesiredService(context.Background(), "web"); err != nil {
		t.Fatalf("delete app: %v", err)
	}
	if code := postApproval(t, rt, b, id, "approve"); code != http.StatusConflict {
		t.Fatalf("approve of a gone app = %d, want 409", code)
	}
	a, _ := db.GetDeployApproval(context.Background(), id)
	if a.Status != store.DeployApprovalStatusPending || a.ApprovedBy != "" {
		t.Fatalf("after failed apply: status %q approver %q, want pending and no approver", a.Status, a.ApprovedBy)
	}
}

func TestDeployApprovalRace_CrashLeftClaimRecovered(t *testing.T) {
	rt, db, _, _, id := raceFixture(t)
	t.Setenv("APP_DEPLOY_APPROVAL_CLAIM_TIMEOUT", "1m")
	ctx := context.Background()
	stale := store.FormatAuditTime(time.Now().Add(-2 * time.Minute))
	if ok, err := db.ClaimDeployApproval(ctx, id, store.PrincipalTypeUser, "someone", "someone", stale); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	n, err := rt.recoverStaleApprovalClaims(ctx)
	if err != nil || n != 1 {
		t.Fatalf("recover = %d %v, want 1", n, err)
	}
	if a, _ := db.GetDeployApproval(ctx, id); a.Status != store.DeployApprovalStatusPending {
		t.Fatalf("unapplied stale claim: status %q, want pending", a.Status)
	}

	fresh := store.FormatAuditTime(time.Now())
	if ok, _ := db.ClaimDeployApproval(ctx, id, store.PrincipalTypeUser, "someone", "someone", fresh); !ok {
		t.Fatal("re-claim after recovery failed")
	}
	if n, _ := rt.recoverStaleApprovalClaims(ctx); n != 0 {
		t.Fatalf("a fresh claim must be left alone, recovered %d", n)
	}
}

func TestDeployApprovalRace_CrashAfterApplyRecoversAsApproved(t *testing.T) {
	rt, db, _, _, id := raceFixture(t)
	t.Setenv("APP_DEPLOY_APPROVAL_CLAIM_TIMEOUT", "1m")
	ctx := context.Background()
	stale := store.FormatAuditTime(time.Now().Add(-2 * time.Minute))
	if ok, _ := db.ClaimDeployApproval(ctx, id, store.PrincipalTypeUser, "someone", "someone", stale); !ok {
		t.Fatal("claim failed")
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	svc.Image = requestPendingApprovalImage
	if err := db.SaveDesiredService(ctx, *svc); err != nil {
		t.Fatalf("save: %v", err)
	}
	attemptID, _ := store.NewDeployAttemptID()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: attemptID, ServiceName: "web", Image: requestPendingApprovalImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("save attempt: %v", err)
	}
	if n, err := rt.recoverStaleApprovalClaims(ctx); err != nil || n != 1 {
		t.Fatalf("recover = %d %v", n, err)
	}
	if a, _ := db.GetDeployApproval(ctx, id); a.Status != store.DeployApprovalStatusApproved || a.ApprovedBy != "someone" {
		t.Fatalf("applied stale claim: status %q approver %q, want approved", a.Status, a.ApprovedBy)
	}
}

// A same-tag redeploy already matches the desired image before it applies,
// so a crash before the apply must return the row to pending.
func TestDeployApprovalRace_SameTagCrashBeforeApplyIsPending(t *testing.T) {
	rt, db, _, _, id := raceFixture(t)
	t.Setenv("APP_DEPLOY_APPROVAL_CLAIM_TIMEOUT", "1m")
	ctx := context.Background()
	svc, _ := db.GetDesiredService(ctx, "web")
	svc.Image = requestPendingApprovalImage
	if err := db.SaveDesiredService(ctx, *svc); err != nil {
		t.Fatalf("save: %v", err)
	}
	stale := store.FormatAuditTime(time.Now().Add(-2 * time.Minute))
	if ok, _ := db.ClaimDeployApproval(ctx, id, store.PrincipalTypeUser, "someone", "someone", stale); !ok {
		t.Fatal("claim failed")
	}
	if _, err := rt.recoverStaleApprovalClaims(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if a, _ := db.GetDeployApproval(ctx, id); a.Status != store.DeployApprovalStatusPending {
		t.Fatalf("same-tag claim with no apply: status %q, want pending", a.Status)
	}
}

package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// movingResolver resolves every tag to a digest a test can re-point.
type movingResolver struct {
	mu     sync.Mutex
	digest string
}

func (m *movingResolver) ResolveImage(_ context.Context, ref string, _ *docker.RegistryAuth, _ bool) (docker.ResolvedImage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return docker.ResolvedImage{Ref: docker.PinImageRef(ref, m.digest), Digest: m.digest, Source: docker.ImageSourceRegistry}, nil
}

func (m *movingResolver) point(c string) {
	m.mu.Lock()
	m.digest = "sha256:" + strings.Repeat(c, 64)
	m.mu.Unlock()
}

// multiFixture: a pending deploy into env_prod needing two approvers, with
// an admin requester and two eligible approvers.
func multiFixture(t *testing.T) (*Router, *store.DB, *movingResolver, *http.Cookie, *http.Cookie, string) {
	t.Helper()
	res := &movingResolver{}
	res.point("a")
	rt, db, requester, b := newProtectedSafetyRouter(t, res)
	c := storeUserWithAbilitiesForTest(t, db, "c@example.com", []string{AbilityRead, AbilityDeploy})
	id := requestPendingApproval(t, rt, requester)
	requireTwoApprovers(t, db)
	return rt, db, res, b, sessionCookieForTest(t, rt, c.ID), id
}

func requireApprovers(t *testing.T, rt *Router, admin *http.Cookie, n string) int {
	t.Helper()
	return serve(rt, authedRequest(t, admin, http.MethodPatch, "/api/v1/environments/env_prod", `{"required_approvers":`+n+`}`)).Code
}

func requireTwoApprovers(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SetEnvironmentRequiredApprovers(context.Background(), "env_prod", 2, "2026-11-09T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
}

func approvalStatus(t *testing.T, db *store.DB, id string) string {
	t.Helper()
	a, err := db.GetDeployApproval(context.Background(), id)
	if err != nil {
		t.Fatalf("load approval: %v", err)
	}
	return a.Status
}

func TestRequiredApprovers_DefaultSingleApproverUnchanged(t *testing.T) {
	rt, db, b, _, id := raceFixture(t)
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusOK {
		t.Fatalf("approve = %d, want 200", got)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusApproved {
		t.Fatalf("status = %s, want approved", s)
	}
}

func TestRequiredApprovers_BoundedByRuntimeMax(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	if got := requireApprovers(t, rt, admin, "11"); got != http.StatusBadRequest {
		t.Fatalf("required 11 = %d, want 400", got)
	}
	if got := requireApprovers(t, rt, admin, "0"); got != http.StatusBadRequest {
		t.Fatalf("required 0 = %d, want 400", got)
	}
}

func TestRequiredApprovers_RequesterCannotSelfApprove(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	if got := requireApprovers(t, rt, admin, "2"); got != http.StatusOK {
		t.Fatalf("set required = %d", got)
	}
	u := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityDeploy})
	id := requestPendingApproval(t, rt, sessionCookieForTest(t, rt, u.ID))
	if got := postApproval(t, rt, sessionCookieForTest(t, rt, u.ID), id, "approve"); got != http.StatusForbidden {
		t.Fatalf("self approve = %d, want 403", got)
	}
	if rec := bearerApprove(rt, seedApprovalToken(t, db, "tok_self", u.ID), id); rec.Code != http.StatusForbidden {
		t.Fatalf("self approve via token = %d, want 403", rec.Code)
	}
}

func TestRequiredApprovers_SameUserTwoTokensCountsOnce(t *testing.T) {
	rt, db, _, _, c, id := multiFixture(t)
	b := storeUserWithAbilitiesForTest(t, db, "twotok@example.com", []string{AbilityRead, AbilityDeploy})
	if rec := bearerApprove(rt, seedApprovalToken(t, db, "tok_b1", b.ID), id); rec.Code != http.StatusAccepted {
		t.Fatalf("first token = %d %s, want 202", rec.Code, rec.Body.String())
	}
	if rec := bearerApprove(rt, seedApprovalToken(t, db, "tok_b2", b.ID), id); rec.Code != http.StatusAccepted {
		t.Fatalf("second token of same user = %d, want 202 (still one approver)", rec.Code)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusPending {
		t.Fatalf("status after one user = %s, want pending", s)
	}
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusOK {
		t.Fatalf("second distinct approver = %d, want 200", got)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusApproved {
		t.Fatalf("status = %s, want approved", s)
	}
}

func TestRequiredApprovers_RevokedApproverDoesNotCount(t *testing.T) {
	rt, db, _, _, c, id := multiFixture(t)
	x := storeUserWithAbilitiesForTest(t, db, "x@example.com", []string{AbilityRead, AbilityDeploy})
	if rec := bearerApprove(rt, seedApprovalToken(t, db, "tok_x", x.ID), id); rec.Code != http.StatusAccepted {
		t.Fatalf("token vote = %d %s", rec.Code, rec.Body.String())
	}
	if err := db.RevokeAPIToken(context.Background(), "tok_x"); err != nil {
		t.Fatal(err)
	}
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("approve after revoke = %d, want 202 (revoked vote ignored)", got)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusPending {
		t.Fatalf("status = %s, want pending", s)
	}
}

func TestRequiredApprovers_NthApprovalReleasesOnceUnderConcurrency(t *testing.T) {
	rt, db, _, b, _, id := multiFixture(t)
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("first vote = %d, want 202", got)
	}
	const racers = 6
	cookies := make([]*http.Cookie, racers)
	for i := range cookies {
		u := storeUserWithAbilitiesForTest(t, db, "racer"+string(rune('a'+i))+"@example.com", []string{AbilityRead, AbilityDeploy})
		cookies[i] = sessionCookieForTest(t, rt, u.ID)
	}
	codes := make([]int, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range cookies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = postApproval(t, rt, cookies[i], id, "approve")
		}(i)
	}
	close(start)
	wg.Wait()
	released := 0
	for _, c := range codes {
		if c == http.StatusOK {
			released++
		}
	}
	if released != 1 {
		t.Fatalf("codes = %v, want exactly one release", codes)
	}
	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, at := range attempts {
		if at.Image == requestPendingApprovalImage {
			n++
		}
	}
	if n > 1 {
		t.Fatalf("deploy attempts for approved image = %d, want at most 1", n)
	}
	if got := countAudit(t, db, "deploy_approval.release"); got != 1 {
		t.Fatalf("release audit entries = %d, want 1", got)
	}
}

func countAudit(t *testing.T, db *store.DB, action string) int {
	t.Helper()
	rows, err := db.ListAuditEntries(context.Background(), 1000, nil, store.AuditEntryFilter{Action: action})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestRequiredApprovers_TagRepointedBetweenVotesRefusesRelease(t *testing.T) {
	rt, db, res, b, c, id := multiFixture(t)
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("first vote = %d, want 202", got)
	}
	res.point("b")
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusConflict {
		t.Fatalf("vote after tag moved = %d, want 409", got)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusPending {
		t.Fatalf("status = %s, want pending", s)
	}
}

func TestRequiredApprovers_ReleaseDeploysPinnedDigest(t *testing.T) {
	rt, db, _, b, c, id := multiFixture(t)
	postApproval(t, rt, b, id, "approve")
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusOK {
		t.Fatalf("second vote = %d, want 200", got)
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil || docker.ImageDigestOf(svc.Image) != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("desired image = %v, %v; want pinned to the voted digest", svc, err)
	}
}

func TestRequiredApprovers_RevokedTokenThenSessionCountsOnce(t *testing.T) {
	rt, db, _, _, c, id := multiFixture(t)
	x := storeUserWithAbilitiesForTest(t, db, "x2@example.com", []string{AbilityRead, AbilityDeploy})
	if rec := bearerApprove(rt, seedApprovalToken(t, db, "tok_x2", x.ID), id); rec.Code != http.StatusAccepted {
		t.Fatalf("token vote = %d", rec.Code)
	}
	if err := db.RevokeAPIToken(context.Background(), "tok_x2"); err != nil {
		t.Fatal(err)
	}
	xs := sessionCookieForTest(t, rt, x.ID)
	if got := postApproval(t, rt, xs, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("session vote after revoke = %d, want 202", got)
	}
	if got := postApproval(t, rt, xs, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("repeat session vote = %d, want 202 (still one)", got)
	}
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusOK {
		t.Fatalf("second person = %d, want 200 (revoked vote replaced, counted once)", got)
	}
}

func TestRequiredApprovers_DemotedApproverStopsCounting(t *testing.T) {
	rt, db, _, b, c, id := multiFixture(t)
	x := storeUserWithAbilitiesForTest(t, db, "x3@example.com", []string{AbilityRead, AbilityDeploy})
	if got := postApproval(t, rt, sessionCookieForTest(t, rt, x.ID), id, "approve"); got != http.StatusAccepted {
		t.Fatalf("vote = %d", got)
	}
	if err := db.UpdateUserAbilities(context.Background(), x.ID, []string{AbilityRead}); err != nil {
		t.Fatal(err)
	}
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("approve after demotion = %d, want 202", got)
	}
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusOK {
		t.Fatalf("third person = %d, want 200", got)
	}
}

func TestRequiredApprovers_OwnerlessTokenRefused(t *testing.T) {
	rt, db, _, _, _, id := multiFixture(t)
	if rec := bearerApprove(rt, seedApprovalToken(t, db, "tok_none", ""), id); rec.Code != http.StatusForbidden {
		t.Fatalf("ownerless token = %d, want 403", rec.Code)
	}
}

func TestRequiredApprovers_WriteOnlyCannotLower(t *testing.T) {
	rt, db, _, _, _, _ := multiFixture(t)
	w := storeUserWithAbilitiesForTest(t, db, "w@example.com", []string{AbilityRead, AbilityWrite})
	wc := sessionCookieForTest(t, rt, w.ID)
	if got := requireApprovers(t, rt, wc, "1"); got != http.StatusForbidden {
		t.Fatalf("write-only lowering = %d, want 403", got)
	}
	if got := requireApprovers(t, rt, wc, "3"); got != http.StatusOK {
		t.Fatalf("write-only raising = %d, want 200", got)
	}
	if got := serve(rt, authedRequest(t, wc, http.MethodPatch, "/api/v1/environments/env_prod", `{"protected":false}`)).Code; got != http.StatusForbidden {
		t.Fatalf("write-only unprotect = %d, want 403", got)
	}
}

func TestRequiredApprovers_ReplacedHeldRequestResetsApprovals(t *testing.T) {
	rt, db, _ := newHoldRouter(t)
	requireTwoApprovers(t, db)
	b := sessionCookieForTest(t, rt, storeUserWithAbilitiesForTest(t, db, "hb@example.com", []string{AbilityRead, AbilityDeploy}).ID)
	c := sessionCookieForTest(t, rt, storeUserWithAbilitiesForTest(t, db, "hc@example.com", []string{AbilityRead, AbilityDeploy}).ID)
	id, attemptID := holdTestDeploy(t, rt, db)
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("first vote = %d, want 202", got)
	}
	// The held request changes under the approval: the earlier vote must not carry over.
	if _, err := db.ExecContext(context.Background(), `UPDATE deploy_attempts SET held_request = ? WHERE id = ?`, `{"kind":"image","image":"levelrail/web:3"}`, attemptID); err != nil {
		t.Fatal(err)
	}
	if got := postApproval(t, rt, c, id, "approve"); got != http.StatusAccepted {
		t.Fatalf("vote after replace = %d, want 202", got)
	}
	if s := approvalStatus(t, db, id); s != store.DeployApprovalStatusPending {
		t.Fatalf("status = %s, want pending", s)
	}
	// A superseded approval is refused outright.
	rt.supersedeHeldDeploys(context.Background(), "web")
	if got := postApproval(t, rt, b, id, "approve"); got != http.StatusConflict {
		t.Fatalf("approve superseded = %d, want 409", got)
	}
	env, err := db.GetEnvironment(context.Background(), "env_prod")
	if err != nil {
		t.Fatal(err)
	}
	next, err := rt.HoldForApproval(context.Background(), env, "web", "levelrail/web:4", store.DeployAttemptSourceWebhook,
		deploy.HeldRequest{Kind: deploy.HeldKindImage, Image: "levelrail/web:4"}, SystemApprovalActor)
	if err != nil {
		t.Fatal(err)
	}
	if got := postApproval(t, rt, b, next, "approve"); got != http.StatusAccepted {
		t.Fatalf("new request first vote = %d, want 202", got)
	}
	if got := postApproval(t, rt, c, next, "approve"); got != http.StatusOK {
		t.Fatalf("new request second vote = %d, want 200", got)
	}
}

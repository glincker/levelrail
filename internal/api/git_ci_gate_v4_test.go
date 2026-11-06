package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

func waitCheckouts(log *gateFetchLog, n int, within time.Duration) []string {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) && len(log.seen()) < n {
		time.Sleep(20 * time.Millisecond)
	}
	return log.seen()
}

func stubBranchHead(rt *Router, sha string) {
	rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) { return sha, nil }
}

func TestCIGateTagPushWithoutCommitIDResolvesTheTagNotTheTagObject(t *testing.T) {
	rt, _, fake, log := gateWebhookRigSafety(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	stubReleaseTag(t, func() (string, error) { return gateSHA, nil })
	const tagObject = "1111111111111111111111111111111111111111"
	body := []byte(fmt.Sprintf(`{"ref":"refs/tags/v1.2.0","after":%q,"before":%q}`, tagObject, zeroGitSHA))
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", releaseGateSource(), body, http.Header{}); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	waitGateBackground(t, rt)
	for _, p := range fake.seen() {
		if strings.Contains(p, tagObject) {
			t.Fatalf("gate read the tag object: %v", fake.seen())
		}
	}
	if got := waitCheckouts(log, 1, 5*time.Second); len(got) != 1 || got[0] != gateSHA {
		t.Fatalf("checkouts = %v, want the resolved commit %s", got, gateSHA)
	}
}

func TestCIGateTagPushWithoutCommitIDSkipsWhenTagCannotBeResolved(t *testing.T) {
	rt, db, _, log := gateWebhookRigSafety(t)
	t.Setenv("APP_CI_GATE_TIMEOUT", "300ms")
	stubReleaseTag(t, func() (string, error) { return "", fmt.Errorf("no such tag") })
	body := []byte(fmt.Sprintf(`{"ref":"refs/tags/v1.2.0","after":%q,"before":%q}`, "1111111111111111111111111111111111111111", zeroGitSHA))
	rt.processGitPushWebhookPayload(t.Context(), "web", releaseGateSource(), body, http.Header{})
	waitGateBackground(t, rt)
	if anyCheckout(log, 300*time.Millisecond) {
		t.Fatalf("deployed an unresolvable tag: %v", log.seen())
	}
	attempts, _ := db.ListDeployAttempts(t.Context(), "web")
	if len(attempts) != 1 || !strings.Contains(attempts[0].Error, "skipped by CI gate") {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestCIGateScheduledDeployOfAnotherBranchDoesNotCancelAWaitingPush(t *testing.T) {
	rt, db, fake, log := gateWebhookRigSafety(t)
	saveGatedGitSource(t, db, gatedPushSource().RepoURL)
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("push status = %d", status)
	}
	stubBranchHead(rt, gateSHA2)
	if _, err := rt.triggerGitDeploy(t.Context(), "web", "release", store.DeployAttemptSourceSchedule); err != nil {
		t.Fatal(err)
	}
	close(block)
	waitGateBackground(t, rt)
	got := waitCheckouts(log, 2, 5*time.Second)
	if len(got) != 2 {
		t.Fatalf("checkouts = %v, want both the push and the other branch", got)
	}
}

func TestCIGateSameRefScheduledDeployRecordsWhyItSupersededAPush(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	saveGatedGitSource(t, db, gatedPushSource().RepoURL)
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	stubBranchHead(rt, gateSHA2)
	if _, err := rt.triggerGitDeploy(t.Context(), "web", "main", store.DeployAttemptSourceSchedule); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		attempts, _ := db.ListDeployAttempts(t.Context(), "web")
		for _, a := range attempts {
			if a.Status == store.DeployAttemptStatusSuperseded {
				close(block)
				waitGateBackground(t, rt)
				if !strings.Contains(a.Reason, store.DeployAttemptSourceSchedule) {
					t.Fatalf("reason = %q, want it to name the scheduled deploy", a.Reason)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			close(block)
			t.Fatal("no superseded row")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCIGateWaitsPerAppAreCapped(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	t.Setenv("APP_CI_GATE_MAX_WAITS_PER_APP", "2")
	saveGatedGitSource(t, db, gatedPushSource().RepoURL)
	block := blockGate(fake)
	defer close(block)
	fake.set(`[{"name":"test","status":"success"}]`)
	stubBranchHead(rt, gateSHA2)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	if _, err := rt.triggerGitDeploy(t.Context(), "web", "release", store.DeployAttemptSourceSchedule); err != nil {
		t.Fatalf("second ref: %v", err)
	}
	stubBranchHead(rt, gateSHA3)
	if _, err := rt.triggerGitDeploy(t.Context(), "web", "dev", store.DeployAttemptSourceSchedule); err == nil {
		t.Fatal("a third waiting ref was accepted past the cap")
	}
}

func TestCIGateShutdownLeavesTheHeadEligibleForThePoller(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	blockGate(fake)
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA2, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBodyBefore(gateSHA, gateSHA2), http.Header{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if !rt.ShutdownCIGates(ctx) {
		t.Fatal("ShutdownCIGates hit its deadline")
	}
	rel, err := db.GetGitSourceReliability(t.Context(), "web")
	if err != nil || rel.LastSeenHead != gateSHA2 {
		t.Fatalf("last seen head = %q, err = %v, want it rolled back to %s", rel.LastSeenHead, err, gateSHA2)
	}
}

func TestCIGateRefusesNewWaitsOnceShutdownBegan(t *testing.T) {
	rt, db, _, log := gateWebhookRigSafety(t)
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA2, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	rt.CancelCIGates()
	status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBodyBefore(gateSHA, gateSHA2), http.Header{})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	rt.ciGates().mu.Lock()
	left := len(rt.ciGates().slots)
	rt.ciGates().mu.Unlock()
	if left != 0 || anyCheckout(log, 100*time.Millisecond) {
		t.Fatalf("a gate started after shutdown: slots = %d", left)
	}
	if rel, _ := db.GetGitSourceReliability(t.Context(), "web"); rel.LastSeenHead != gateSHA2 {
		t.Fatalf("last seen head = %q, a refused push must stay eligible for the poller", rel.LastSeenHead)
	}
}

func TestCIGateOverTheCapPushRestoresTheHeadAndLaterPollGatesIt(t *testing.T) {
	rt, db, fake, log := gateWebhookRigSafety(t)
	t.Setenv("APP_CI_GATE_MAX_WAITS_PER_APP", "1")
	saveGatedGitSource(t, db, gatedPushSource().RepoURL)
	secrets, _ := rt.gitSourceSecrets.(*fakeGitSourceSecrets)
	if err := secrets.SetValue(t.Context(), store.GitSourceSecretsKey("web"), gitSourceSecretKey, "s3cret"); err != nil {
		t.Fatal(err)
	}
	block := blockGate(fake)
	var once sync.Once
	release := func() { once.Do(func() { close(block) }) }
	t.Cleanup(release)
	fake.set(`[{"name":"test","status":"success"}]`)
	stubBranchHead(rt, gateSHA2)
	if _, err := rt.triggerGitDeploy(t.Context(), "web", "release", store.DeployAttemptSourceSchedule); err != nil {
		t.Fatal(err)
	}
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA3, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(gitLabPushBodyBefore(gateSHA, gateSHA3))))
	req.SetPathValue("name", "web")
	req.Header.Set(webhook.HeaderGitLabToken, "s3cret")
	rec := httptest.NewRecorder()
	rt.handleGitPushWebhook(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rel, _ := db.GetGitSourceReliability(t.Context(), "web"); rel.LastSeenHead != gateSHA3 {
		t.Fatalf("last seen head = %q, want it restored to %s", rel.LastSeenHead, gateSHA3)
	}
	release()
	waitGateBackground(t, rt)
	if err := rt.handlePolledHead(t.Context(), "web", gatedPushSource(), gateSHA); err != nil {
		t.Fatal(err)
	}
	waitGateBackground(t, rt)
	got := waitCheckouts(log, 2, 5*time.Second)
	found := false
	for _, s := range got {
		found = found || s == gateSHA
	}
	if !found {
		t.Fatalf("checkouts = %v, the refused push was never gated by the poller", got)
	}
}

func TestCIGateShutdownRollbackDoesNotReopenAHeadThatMovedOn(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	blockGate(fake)
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA2, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBodyBefore(gateSHA, gateSHA2), http.Header{})
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA3, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	rt.ShutdownCIGates(ctx)
	if rel, _ := db.GetGitSourceReliability(t.Context(), "web"); rel.LastSeenHead != gateSHA3 {
		t.Fatalf("last seen head = %q, a rollback must not overwrite a newer head", rel.LastSeenHead)
	}
}

func TestCIGateRefSpellingsShareOneWaitKey(t *testing.T) {
	for in, want := range map[string]string{
		"main": "refs/heads/main", "": "refs/heads/main", "refs/heads/main": "refs/heads/main",
		"refs/heads/": "refs/heads/main", "release/1": "refs/heads/release/1", "refs/tags/v1": "refs/tags/v1",
	} {
		if got := gateRef(in); got != want {
			t.Errorf("gateRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCIGateScheduledFullRefSupersedesTheWaitingPushOfThatBranch(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	saveGatedGitSource(t, db, gatedPushSource().RepoURL)
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	rt.processGitPushWebhookPayload(t.Context(), "web", gatedPushSource(), gitLabPushBody("refs/heads/main", gateSHA, ""), http.Header{})
	stubBranchHead(rt, gateSHA2)
	if _, err := rt.triggerGitDeploy(t.Context(), "web", "refs/heads/main", store.DeployAttemptSourceSchedule); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		attempts, _ := db.ListDeployAttempts(t.Context(), "web")
		for _, a := range attempts {
			if a.Status == store.DeployAttemptStatusSuperseded {
				close(block)
				waitGateBackground(t, rt)
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(block)
	t.Fatal("the same branch spelled as a full ref did not supersede the waiting push")
}

func TestCIGateTagResolveShutdownSaysNothingRetriesIt(t *testing.T) {
	rt, db, _, _ := gateWebhookRigSafety(t)
	stubReleaseTag(t, func() (string, error) { return "", fmt.Errorf("no such tag") })
	body := []byte(fmt.Sprintf(`{"ref":"refs/tags/v1.2.0","after":%q,"before":%q}`, "1111111111111111111111111111111111111111", zeroGitSHA))
	rt.processGitPushWebhookPayload(t.Context(), "web", releaseGateSource(), body, http.Header{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	rt.ShutdownCIGates(ctx)
	attempts, _ := db.ListDeployAttempts(t.Context(), "web")
	if len(attempts) != 1 || !strings.Contains(attempts[0].Error, "nothing retries it") {
		t.Fatalf("attempts = %+v", attempts)
	}
}

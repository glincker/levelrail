package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gateSHA3 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var releaseBodyV1 = []byte(`{"action":"published","release":{"tag_name":"v1.0.0"}}`)

type gateAttemptNotifier struct {
	mu  sync.Mutex
	ids []string
}

func (n *gateAttemptNotifier) Dispatch(_ context.Context, _ string, ev alerting.DeployOutcome) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ids = append(n.ids, ev.AttemptID)
}

func (n *gateAttemptNotifier) notified(id string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, got := range n.ids {
		if got == id {
			return true
		}
	}
	return false
}

func releaseGateSource() store.GitSource {
	gs := gatedPushSource()
	gs.TriggerMode = spec.TriggerModeRelease
	return gs
}

// stubReleaseTag swaps the tag resolver; resolve may fail a few times first.
func stubReleaseTag(t *testing.T, resolve func() (string, error)) {
	t.Helper()
	prev := resolveReleaseTagSHA
	resolveReleaseTagSHA = func(context.Context, string, string, string) (string, error) { return resolve() }
	t.Cleanup(func() { resolveReleaseTagSHA = prev })
}

func saveGatedGitSource(t *testing.T, db *store.DB, repoURL string) {
	t.Helper()
	if err := db.SaveGitSource(t.Context(), store.GitSource{ServiceName: "web", RepoURL: repoURL, Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitSourceCIGate(t.Context(), "web", true, []string{"test"}); err != nil {
		t.Fatal(err)
	}
}

func TestGitWebhookCIGate_SupersededWaitIsInfoRowWithoutNotification(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	notifier := &gateAttemptNotifier{}
	rt.deployNotifier = notifier
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	gs := gatedPushSource()
	for _, sha := range []string{gateSHA, gateSHA2} {
		if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBody("refs/heads/main", sha, ""), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("status = %d", status)
		}
	}
	var superseded store.DeployAttempt
	deadline := time.Now().Add(5 * time.Second)
	for superseded.ID == "" {
		attempts, _ := db.ListDeployAttempts(t.Context(), "web")
		for _, a := range attempts {
			if a.Status == store.DeployAttemptStatusSuperseded {
				superseded = a
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no superseded row: %+v", attempts)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if superseded.CommitSHA != gateSHA || superseded.Source != store.DeployAttemptSourceWebhook {
		t.Errorf("superseded row = %+v", superseded)
	}
	close(block)
	waitGateBackground(t, rt)
	if notifier.notified(superseded.ID) {
		t.Fatal("a superseded wait sent a deploy failed notification")
	}
}

func TestGitWebhookCIGate_ForcePushBackToOlderCommitSupersedesTheWaitingOne(t *testing.T) {
	rt, _, fake, log := gateWebhookRigSafety(t)
	block := blockGate(fake)
	fake.set(`[{"name":"test","status":"success"}]`)
	gs := gatedPushSource()
	// A (gateSHA2) is waiting, a child of C (gateSHA); the force push undoes A by pointing back at C.
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBodyBefore(gateSHA2, gateSHA), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("A status = %d", status)
	}
	status, msg := rt.processGitPushWebhookPayload(t.Context(), "web", gs, gitLabPushBodyBefore(gateSHA, gateSHA2), http.Header{})
	if status != http.StatusAccepted {
		t.Fatalf("force push status = %d, msg = %q, want it to supersede A", status, msg)
	}
	close(block)
	waitGateBackground(t, rt)
	if !anyCheckout(log, 5*time.Second) {
		t.Fatal("nothing deployed")
	}
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA {
		t.Fatalf("checkouts = %v, want only the force-pushed %s", got, gateSHA)
	}
}

func TestGitWebhookCIGate_GitHubAnnotatedTagGatesTheCommitNotTheTagObject(t *testing.T) {
	rt, _, fake, log := gateWebhookRigSafety(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	const tagObject = "1111111111111111111111111111111111111111"
	body := []byte(fmt.Sprintf(`{"ref":"refs/tags/v1.2.0","after":%q,"before":%q,"head_commit":{"id":%q}}`, tagObject, zeroGitSHA, gateSHA))
	if status, _ := rt.processGitPushWebhookPayload(t.Context(), "web", releaseGateSource(), body, http.Header{}); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	waitGateBackground(t, rt)
	seen := fake.seen()
	if len(seen) == 0 || !strings.Contains(seen[0], gateSHA) || strings.Contains(seen[0], tagObject) {
		t.Fatalf("paths = %v, want the commit %s", seen, gateSHA)
	}
	if !anyCheckout(log, 5*time.Second) {
		t.Fatal("nothing deployed")
	}
	if got := log.seen(); len(got) != 1 || got[0] != gateSHA {
		t.Fatalf("checkouts = %v, want the gated commit %s", got, gateSHA)
	}
}

func TestGitWebhookCIGate_ReleaseEventGateDecidesThePinnedSHA(t *testing.T) {
	for _, tc := range []struct {
		status string
		deploy bool
	}{{"success", true}, {"failed", false}} {
		rt, _, fake, log := gateWebhookRigSafety(t)
		fake.set(`[{"name":"test","status":"` + tc.status + `"}]`)
		calls := 0
		// The tag is not visible for the first two reads: the job retries instead of giving up.
		stubReleaseTag(t, func() (string, error) {
			calls++
			if calls < 3 {
				return "", fmt.Errorf("tag not found yet")
			}
			return gateSHA, nil
		})
		start := time.Now()
		status, _ := rt.processGitHubReleaseWebhookEvent(t.Context(), "web", releaseGateSource(), releaseBodyV1)
		if status != http.StatusAccepted || time.Since(start) > time.Second {
			t.Fatalf("%s: status = %d after %s, want an immediate 202", tc.status, status, time.Since(start))
		}
		waitGateBackground(t, rt)
		got := anyCheckout(log, time.Second)
		if got != tc.deploy {
			t.Fatalf("%s: deployed = %v, want %v", tc.status, got, tc.deploy)
		}
		if tc.deploy && log.seen()[0] != gateSHA {
			t.Fatalf("checkouts = %v, want the pinned %s", log.seen(), gateSHA)
		}
	}
}

func TestGitWebhookCIGate_ReleaseEventDoesNotRedeployTheCommitATagPushDeployed(t *testing.T) {
	rt, db, fake, log := gateWebhookRigSafety(t)
	fake.set(`[{"name":"test","status":"success"}]`)
	stubReleaseTag(t, func() (string, error) { return gateSHA, nil })
	if err := db.SaveDeployAttempt(t.Context(), store.DeployAttempt{
		ID: "att-tag-push", ServiceName: "web", Image: "web:" + gateSHA, CommitSHA: gateSHA,
		Source: store.DeployAttemptSourceWebhook, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if status, _ := rt.processGitHubReleaseWebhookEvent(t.Context(), "web", releaseGateSource(), releaseBodyV1); status != http.StatusAccepted {
		t.Fatalf("status = %d", status)
	}
	waitGateBackground(t, rt)
	if anyCheckout(log, 500*time.Millisecond) {
		t.Fatalf("release event deployed a commit the tag push already deployed: %v", log.seen())
	}
}

func TestPolledHeadStaysUnnotedWhenTheWaitIsInterrupted(t *testing.T) {
	rt, db, fake, log := gateWebhookRigSafety(t)
	blockGate(fake)
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA2, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := rt.handlePolledHead(t.Context(), "web", gatedPushSource(), gateSHA); err != nil {
		t.Fatal(err)
	}
	rt.CancelCIGates()
	waitGateBackground(t, rt)
	rel, err := db.GetGitSourceReliability(t.Context(), "web")
	if err != nil || rel.LastSeenHead != gateSHA2 {
		t.Fatalf("last seen head = %q, err = %v: an interrupted head must stay for the next poll", rel.LastSeenHead, err)
	}
	if anyCheckout(log, 200*time.Millisecond) {
		t.Fatal("deployed after shutdown")
	}
}

func TestPolledHeadIsNotedAfterARedGateAndLabelledAsPoll(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	fake.set(`[{"name":"test","status":"failed"}]`)
	if err := db.NoteGitSourceHead(t.Context(), "web", gateSHA2, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if err := rt.handlePolledHead(t.Context(), "web", gatedPushSource(), gateSHA); err != nil {
		t.Fatal(err)
	}
	waitGateBackground(t, rt)
	rel, err := db.GetGitSourceReliability(t.Context(), "web")
	if err != nil || rel.LastSeenHead != gateSHA {
		t.Fatalf("last seen head = %q, err = %v, want the red head remembered", rel.LastSeenHead, err)
	}
	attempts, _ := db.ListDeployAttempts(t.Context(), "web")
	if len(attempts) != 1 || attempts[0].Source != store.DeployAttemptSourcePoll {
		t.Fatalf("attempts = %+v, want one skip row sourced from the poller", attempts)
	}
}

func TestScheduledAndHookBranchDeploysAreGated(t *testing.T) {
	for _, source := range []string{store.DeployAttemptSourceSchedule, store.DeployAttemptSourceWebhook, store.DeployAttemptSourcePRComment} {
		for _, status := range []string{"failed", "success"} {
			rt, db, fake, log := gateWebhookRigSafety(t)
			saveGatedGitSource(t, db, gatedPushSource().RepoURL)
			fake.set(`[{"name":"test","status":"` + status + `"}]`)
			rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) { return gateSHA, nil }
			msg, err := rt.triggerGitDeploy(t.Context(), "web", "main", source)
			if err != nil || !strings.HasPrefix(msg, gatedAcceptedPrefix) {
				t.Fatalf("%s/%s: msg = %q, err = %v, want the deploy accepted behind the gate", source, status, msg, err)
			}
			waitGateBackground(t, rt)
			deployed := anyCheckout(log, time.Second)
			if deployed != (status == "success") {
				t.Fatalf("%s/%s: deployed = %v", source, status, deployed)
			}
			if status == "failed" {
				attempts, _ := db.ListDeployAttempts(t.Context(), "web")
				if len(attempts) != 1 || attempts[0].Source != source || !strings.Contains(attempts[0].Error, "skipped by CI gate") {
					t.Fatalf("%s: attempts = %+v, want one gate skip with the real source", source, attempts)
				}
			}
		}
	}
}

func TestCIGateRefusesPlainHTTPRepoURLs(t *testing.T) {
	rt, db, fake, _ := gateWebhookRigSafety(t)
	saveGatedGitSource(t, db, "http://gitlab.example.com/org/web.git")
	if status, _ := rt.ciGateUnavailable(t.Context(), "web"); status != http.StatusUnprocessableEntity {
		t.Fatalf("ciGateUnavailable status = %d, want 422", status)
	}
	gs := gitlabGateSource("org/web", "test")
	gs.RepoURL = "http://gitlab.example.com/org/web.git"
	if msg := gateOnCIWeb(t.Context(), rt, gs, gateSHA); !strings.HasPrefix(msg, "not deployed: ") {
		t.Fatalf("gate over http let the commit through: %q", msg)
	}
	if got := fake.seen(); len(got) != 0 {
		t.Fatalf("the CI read went out over http: %v", got)
	}
}

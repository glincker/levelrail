package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

func TestRepoIdentity(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Org/Web.git":          "org/web",
		"git@github.com:org/web.git":              "org/web",
		"ssh://git@github.com/org/web":            "org/web",
		"org/web":                                 "org/web",
		"https://gitlab.com/group/sub/proj.git/":  "group/sub/proj",
		"https://user:tok@github.com/org/web.git": "org/web",
		"": "",
	}
	for in, want := range cases {
		if got := repoIdentity(in); got != want {
			t.Errorf("repoIdentity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPushRepoMatches(t *testing.T) {
	tracked := "https://github.com/org/web.git"
	cases := []struct {
		name string
		ev   webhook.PushEvent
		want bool
	}{
		{"no repo info", webhook.PushEvent{}, true},
		{"same full_name", webhook.PushEvent{RepoNames: []string{"org/web"}}, true},
		{"case and clone url", webhook.PushEvent{RepoNames: []string{"Org/Web"}, RepoURLs: []string{"https://github.com/Org/Web.git"}}, true},
		{"ssh url only", webhook.PushEvent{RepoURLs: []string{"git@github.com:org/web.git"}}, true},
		{"fork", webhook.PushEvent{RepoNames: []string{"someone/web"}, RepoURLs: []string{"https://github.com/someone/web.git"}}, false},
		{"renamed repo", webhook.PushEvent{RepoNames: []string{"org/web-v2"}}, false},
		{"unrelated", webhook.PushEvent{RepoNames: []string{"someone-else/other-repo"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := pushRepoMatches(tc.ev, tracked); got != tc.want {
				t.Errorf("match = %v, want %v", got, tc.want)
			}
		})
	}
	if ok, _ := pushRepoMatches(webhook.PushEvent{RepoNames: []string{"Group/Sub/Proj"}}, "git@gitlab.com:group/sub/proj.git"); !ok {
		t.Error("nested gitlab group should match across ssh and case")
	}
}

func TestWebhookDeliveryIDHeaders(t *testing.T) {
	for _, h := range []string{webhook.HeaderGitHubDelivery, webhook.HeaderGitLabEventUUID, webhook.HeaderGiteaDelivery, webhook.HeaderBitbucketRequest} {
		hd := http.Header{}
		hd.Set(h, "abc-123")
		if got := webhookDeliveryID(hd); got != "abc-123" {
			t.Errorf("%s: got %q", h, got)
		}
	}
	if got := webhookDeliveryID(http.Header{}); got != "" {
		t.Errorf("empty header gave %q", got)
	}
}

func TestParsePushEventRepoFields(t *testing.T) {
	ev, err := webhook.ParsePushEvent([]byte(`{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"org/web","clone_url":"https://github.com/org/web.git"}}`))
	if err != nil || len(ev.RepoNames) != 1 || ev.RepoNames[0] != "org/web" || len(ev.RepoURLs) != 1 {
		t.Fatalf("ev = %+v, err = %v", ev, err)
	}
}

func pushPayloadFor(after, fullName string) []byte {
	return []byte(`{"ref":"refs/heads/main","before":"aaa","after":"` + after + `","repository":{"full_name":"` + fullName + `"}}`)
}

func TestProcessPushRepoMismatchIgnoredWithoutAttempt(t *testing.T) {
	rt, db, _, gs := newWebhookQueueRouter(t)
	status, msg := rt.processGitPushWebhookPayload(context.Background(), "web", gs, pushPayloadFor("deadbeef", "someone/other"), http.Header{})
	if status != http.StatusOK || !strings.Contains(msg, "ignored: push is for someone/other, this app tracks org/web") {
		t.Fatalf("got %d %q", status, msg)
	}
	if attempts, _ := db.ListDeployAttempts(context.Background(), "web"); len(attempts) != 0 {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestProcessPushSameCommitStates(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		wantStatus int
		wantMsg    string
	}{
		{"succeeded", store.DeployAttemptStatusSucceeded, http.StatusOK, "already deployed"},
		{"running", store.DeployAttemptStatusRunning, http.StatusOK, "already in flight"},
		{"queued", store.DeployAttemptStatusQueued, http.StatusOK, "already in flight"},
		{"failed retries", store.DeployAttemptStatusFailed, http.StatusAccepted, "deploy queued:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, _, gs := newWebhookQueueRouter(t)
			ctx := context.Background()
			id, _ := store.NewDeployAttemptID()
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", CommitSHA: "cafe0123456789", Source: store.DeployAttemptSourceWebhook, Status: tc.status, StartedAt: time.Now().Add(-time.Minute)}); err != nil {
				t.Fatal(err)
			}
			status, msg := rt.processGitPushWebhookPayload(ctx, "web", gs, pushPayloadFor("cafe0123456789", "org/web"), http.Header{})
			if status != tc.wantStatus || !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("got %d %q", status, msg)
			}
		})
	}
}

func TestProcessPushReplayBypassesDedupe(t *testing.T) {
	rt, db, _, gs := newWebhookQueueRouter(t)
	ctx := context.Background()
	id, _ := store.NewDeployAttemptID()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", CommitSHA: "cafe", Source: store.DeployAttemptSourceWebhook, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	status, _ := rt.processGitPushWebhookPayload(withWebhookReplay(ctx), "web", gs, pushPayloadFor("cafe", "org/web"), http.Header{})
	if status != http.StatusAccepted {
		t.Fatalf("replay status = %d", status)
	}
}

func TestProcessPushReturnsQuicklyWith202AndRunsInBackground(t *testing.T) {
	rt, db, gb, gs := newWebhookQueueRouter(t)
	ctx := context.Background()
	pctx, outcome := withWebhookOutcome(ctx)
	start := time.Now()
	status, msg := rt.processGitPushWebhookPayload(pctx, "web", gs, pushPayloadFor("f00d", "org/web"), http.Header{})
	if status != http.StatusAccepted || !strings.HasPrefix(msg, "deploy queued: "+outcome.AttemptID) || outcome.AttemptID == "" {
		t.Fatalf("got %d %q outcome=%+v", status, msg, outcome)
	}
	if time.Since(start) > time.Second {
		t.Fatal("handler blocked on the build")
	}
	if a := attemptStatus(t, db, outcome.AttemptID); a.Status != store.DeployAttemptStatusQueued {
		t.Fatalf("status before kick = %q", a.Status)
	}
	rt.kickDeployQueue(ctx)
	if got := gb.awaitStart(t); got != outcome.AttemptID {
		t.Fatalf("build ran as %s", got)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, outcome.AttemptID, store.DeployAttemptStatusSucceeded)
}

func TestUnknownCommitIs422WithoutAttemptRow(t *testing.T) {
	rt, db, _, gs := newWebhookQueueRouter(t)
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), errGitCommitNotFound)
	status, msg := rt.deployFromGitSource(context.Background(), "web", gs, "deadbeef", "deadbeef", nil)
	if status != http.StatusUnprocessableEntity || msg != "commit not found" {
		t.Fatalf("got %d %q", status, msg)
	}
	if attempts, _ := db.ListDeployAttempts(context.Background(), "web"); len(attempts) != 0 {
		t.Fatalf("attempts = %+v", attempts)
	}
	if !errors.Is(errGitCommitNotFound, errGitCommitNotFound) {
		t.Fatal("sentinel")
	}
}

func TestDuplicateWebhookDeliveryAndOutcome(t *testing.T) {
	rt, db, _, _ := newWebhookQueueRouter(t)
	ctx := context.Background()
	if err := db.SaveWebhookDelivery(ctx, store.WebhookDelivery{ID: "whd_1", ServiceName: "web", SignatureValid: true, StatusCode: 202, DeliveryID: "g-1", AttemptID: "dep_x", ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if rt.duplicateWebhookDelivery(ctx, "web", "g-1") == nil {
		t.Fatal("expected duplicate")
	}
	if rt.duplicateWebhookDelivery(ctx, "web", "g-2") != nil || rt.duplicateWebhookDelivery(ctx, "other", "g-1") != nil {
		t.Fatal("unexpected duplicate")
	}
	rt.recordWebhookDeliveryOutcome(ctx, "dep_x", store.DeployAttemptStatusFailed, "boom")
	d, _ := db.GetWebhookDelivery(ctx, "whd_1")
	if !strings.Contains(d.Error, "boom") {
		t.Fatalf("error = %q", d.Error)
	}
	if rt.duplicateWebhookDelivery(ctx, "web", "g-1") != nil {
		t.Fatal("a failed original must be reprocessed on redelivery")
	}
	rt.recordWebhookDeliveryOutcome(ctx, "dep_x", store.DeployAttemptStatusSucceeded, "")
	d, _ = db.GetWebhookDelivery(ctx, "whd_1")
	if d.Note != webhookNoteDeployed || d.Error != "" {
		t.Fatalf("delivery = %+v", d)
	}
}

package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newWebhookQueueRouter(t *testing.T) (*Router, *store.DB, *gateBuilder, store.GitSource) {
	t.Helper()
	rt, db, _, gb := newQueueRouter(t)
	gs := store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}
	if err := db.SaveGitSource(context.Background(), gs); err != nil {
		t.Fatal(err)
	}
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	return rt, db, gb, gs
}

func pushPayload(after string) []byte {
	return []byte(`{"ref":"refs/heads/main","before":"aaa","after":"` + after + `"}`)
}

func TestWebhookPushQueuesBehindRunningDeployThenRuns(t *testing.T) {
	rt, db, gb, gs := newWebhookQueueRouter(t)
	ctx := context.Background()
	_, cookie := rt, loginTestSession(t, rt, db)
	running := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)

	status, msg := rt.processGitPushWebhookPayload(ctx, "web", gs, pushPayload("bbb"), http.Header{})
	if status != http.StatusAccepted || !strings.HasPrefix(msg, "queued:") {
		t.Fatalf("push while busy = %d %q", status, msg)
	}
	queued, err := db.ListQueuedDeployAttempts(ctx)
	if err != nil || len(queued) != 1 || queued[0].CommitSHA != "bbb" || queued[0].Source != store.DeployAttemptSourceWebhook {
		t.Fatalf("queued = %+v, %v", queued, err)
	}

	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, running.ID, store.DeployAttemptStatusSucceeded)
	if got := gb.awaitStart(t); got != queued[0].ID {
		t.Fatalf("queued push ran as %s, want the queued row %s", got, queued[0].ID)
	}
	gb.release <- struct{}{}
	a := awaitAttemptStatus(t, db, queued[0].ID, store.DeployAttemptStatusSucceeded)
	if a.FinishedAt == nil {
		t.Fatalf("attempt = %+v", a)
	}
}

func TestWebhookPushWhenIdleRunsAtOnceAndClearsStartingMark(t *testing.T) {
	rt, db, gb, gs := newWebhookQueueRouter(t)
	done := make(chan int, 1)
	go func() {
		status, _ := rt.processGitPushWebhookPayload(context.Background(), "web", gs, pushPayload("ccc"), http.Header{})
		done <- status
	}()
	id := gb.awaitStart(t)
	if id == "" {
		t.Fatal("no attempt id")
	}
	gb.release <- struct{}{}
	if status := <-done; status >= http.StatusBadRequest {
		t.Fatalf("status = %d", status)
	}
	awaitAttemptStatus(t, db, id, store.DeployAttemptStatusSucceeded)
	rt.buildStartMu.Lock()
	defer rt.buildStartMu.Unlock()
	if len(rt.startingDeploys) != 0 {
		t.Fatalf("starting marks leaked: %v", rt.startingDeploys)
	}
}

func TestWebhookPushSupersedesQueuedPushOfSameBranchWhenEnabled(t *testing.T) {
	rt, db, gb, gs := newWebhookQueueRouter(t)
	ctx := context.Background()
	cookie := loginTestSession(t, rt, db)
	if err := db.SetServiceCancelSuperseded(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	running := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	rt.processGitPushWebhookPayload(ctx, "web", gs, pushPayload("b1"), http.Header{})
	rt.processGitPushWebhookPayload(ctx, "web", gs, pushPayload("b2"), http.Header{})

	queued, _ := db.ListQueuedDeployAttempts(ctx)
	if len(queued) != 1 || queued[0].CommitSHA != "b2" {
		t.Fatalf("queued = %+v", queued)
	}
	attempts, _ := db.ListDeployAttempts(ctx, "web")
	superseded := 0
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSuperseded && a.CommitSHA == "b1" && a.SupersededBy == queued[0].ID {
			superseded++
		}
	}
	if superseded != 1 {
		t.Fatalf("attempts = %+v", attempts)
	}
	if s := attemptStatus(t, db, running.ID).Status; s != store.DeployAttemptStatusRunning {
		t.Fatalf("running deploy touched: %q", s)
	}
	gb.release <- struct{}{}
	awaitAttemptStatus(t, db, running.ID, store.DeployAttemptStatusSucceeded)
	gb.awaitStart(t)
	gb.release <- struct{}{}
}

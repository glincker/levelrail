package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

type pinnedHoldEnv struct {
	rt       *Router
	db       *store.DB
	approver *http.Cookie
	fetches  []fakeGitSourceFetchCall
	head     string
}

func newPinnedHoldEnv(t *testing.T, head string) *pinnedHoldEnv {
	t.Helper()
	e := &pinnedHoldEnv{head: head}
	e.db = openTestDB(t)
	b := newFakeBuilder("web:x", nil)
	e.rt = NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), e.db, WithDeploySafety(e.db, nil), WithBuilder(b))
	seedProtectedDeployFixture(t, e.db)
	gs := store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}
	if err := e.db.SaveGitSource(context.Background(), gs); err != nil {
		t.Fatal(err)
	}
	e.rt.gitSourceSecrets = newFakeGitSourceSecrets()
	e.rt.gitSourceFetch = newFakeGitSourceFetch(&e.fetches, t.TempDir(), new(bool), nil)
	e.rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) { return e.head, nil }
	user := storeUserWithAbilitiesForTest(t, e.db, "approver@example.com", []string{AbilityRead, AbilityDeploy})
	e.approver = sessionCookieForTest(t, e.rt, user.ID)
	return e
}

func (e *pinnedHoldEnv) approveOnly(t *testing.T) {
	t.Helper()
	pending, err := e.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v; want one", pending, err)
	}
	if rec := serve(e.rt, authedRequest(t, e.approver, http.MethodPost, "/api/v1/deploy-approvals/"+pending[0].ID+"/approve", "")); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
}

func TestHeldBuildHookReplaysTriggeredCommitWithArgs(t *testing.T) {
	first, moved := strings.Repeat("1", 40), strings.Repeat("2", 40)
	e := newPinnedHoldEnv(t, first)
	hook := store.BuildHook{ID: "bh_1", ServiceName: "web", Name: "cms", Branch: "main"}
	res := e.rt.executeBuildHook(context.Background(), hook, buildHookPayload{Title: "post one", Body: "text"})
	if !strings.HasPrefix(res, "held: ") {
		t.Fatalf("hook result = %q, want held", res)
	}
	e.head = moved
	e.approveOnly(t)
	if len(e.fetches) != 1 || e.fetches[0].sha != first {
		t.Fatalf("fetches = %+v, want the commit seen at hold time %s", e.fetches, first)
	}
	b, _ := e.rt.builder.(*fakeBuilder)
	b.mu.Lock()
	args := b.lastReq.Service.Build.Args
	b.mu.Unlock()
	if args[buildHookArgTitle] != "post one" || args[buildHookArgBody] != "text" || args[buildHookArgBranch] != "main" || args[buildHookArgHook] != "cms" {
		t.Fatalf("build args = %v, want the trigger args", args)
	}
}

func TestHeldReleaseTagReplaysTagCommit(t *testing.T) {
	tagSHA := strings.Repeat("3", 40)
	e := newPinnedHoldEnv(t, strings.Repeat("4", 40))
	prev := resolveTagSHA
	resolveTagSHA = func(context.Context, string, string, string) (string, error) { return tagSHA, nil }
	t.Cleanup(func() { resolveTagSHA = prev })
	gs := store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", TriggerMode: "release"}
	status, _ := e.rt.processGitHubReleaseWebhookEvent(context.Background(), "web", gs, []byte(`{"action":"published","release":{"tag_name":"v1"}}`))
	if status != http.StatusAccepted {
		t.Fatalf("release status = %d", status)
	}
	e.approveOnly(t)
	if len(e.fetches) != 1 || e.fetches[0].sha != tagSHA {
		t.Fatalf("fetches = %+v, want tag commit %s", e.fetches, tagSHA)
	}
}

func TestPolledHeadOnProtectedAppIsHeldAtSeenCommit(t *testing.T) {
	first, second, moved := strings.Repeat("5", 40), strings.Repeat("6", 40), strings.Repeat("7", 40)
	e := newPinnedHoldEnv(t, first)
	ctx := context.Background()
	gs, err := e.db.GetGitSource(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.rt.handlePolledHead(ctx, "web", *gs, first); err != nil {
		t.Fatal(err)
	}
	if err := e.rt.handlePolledHead(ctx, "web", *gs, second); err != nil {
		t.Fatal(err)
	}
	attempts := heldAttempts(t, e.db)
	if len(attempts) != 1 || attempts[0].Status != store.DeployAttemptStatusAwaitingApproval || attempts[0].Source != store.DeployAttemptSourcePoll || attempts[0].CommitSHA != second {
		t.Fatalf("attempts = %+v, want one polled awaiting_approval at %s", attempts, second)
	}
	rel, err := e.db.GetGitSourceReliability(ctx, "web")
	if err != nil || rel.LastSeenHead != second {
		t.Fatalf("last seen head = %+v, %v; want %s", rel, err, second)
	}
	e.head = moved
	e.approveOnly(t)
	if len(e.fetches) != 1 || e.fetches[0].sha != second {
		t.Fatalf("fetches = %+v, want %s", e.fetches, second)
	}
}

func heldRefs(t *testing.T, rt *Router, app string) []string {
	t.Helper()
	repo, _, release, err := rt.sourceCache.OpenMirror(gitPushMirrorURL(app))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	iter, err := repo.Storer.IterReferences()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	_ = iter.ForEach(func(r *plumbing.Reference) error {
		if strings.HasPrefix(r.Name().String(), heldRefPrefix) {
			out = append(out, r.Name().String())
		}
		return nil
	})
	return out
}

func TestGitPushToProtectedAppIsHeldThenBuiltFromMirror(t *testing.T) {
	enableAllExperimental()
	db := openTestDB(t)
	b := &pushTestBuilder{fakeBuilder: newFakeBuilder("web:x", nil)}
	cache := sourcecache.New(sourcecache.Config{Dir: t.TempDir()}, nil)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(b), WithSourceCache(cache), WithDeploySafety(db, nil))
	cookie := loginTestSession(t, rt, db)
	seedProtectedDeployFixture(t, db)
	e := newPushEnvFor(t, rt, db, cookie, b)
	token := e.issue(t, "web")

	repo, first := pushWorkRepo(t, map[string]string{"hello.txt": "one"})
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("push: %v", err)
	}
	if b.callCount() != 0 {
		t.Fatalf("builder ran %d times before approval", b.callCount())
	}
	attempts := heldAttempts(t, db)
	if len(attempts) != 1 || attempts[0].Status != store.DeployAttemptStatusAwaitingApproval || attempts[0].CommitSHA != first.String() || attempts[0].Source != store.DeployAttemptSourceGitPush {
		t.Fatalf("attempts = %+v, want one awaiting_approval push at %s", attempts, first)
	}
	if refs := heldRefs(t, rt, "web"); len(refs) != 1 {
		t.Fatalf("held refs = %v, want one", refs)
	}

	approver := sessionCookieForTest(t, rt, storeUserWithAbilitiesForTest(t, db, "approver@example.com", []string{AbilityRead, AbilityDeploy}).ID)
	pending, err := db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	if rec := serve(rt, authedRequest(t, approver, http.MethodPost, "/api/v1/deploy-approvals/"+pending[0].ID+"/approve", "")); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", rec.Code, rec.Body.String())
	}
	if b.callCount() != 1 || b.files["hello.txt"] != "one" {
		t.Fatalf("after approval calls=%d files=%v, want the pushed content", b.callCount(), b.files)
	}
	if refs := heldRefs(t, rt, "web"); len(refs) != 0 {
		t.Fatalf("held refs after approval = %v, want none", refs)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(advertisedRefs(t, e, token), first.String()) {
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(advertisedRefs(t, e, token), first.String()) {
		t.Fatal("approved branch head is not advertised for the next push")
	}
}

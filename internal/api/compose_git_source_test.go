package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidateComposeRepoPath(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "", want: "compose.yaml"},
		{in: "docker/compose.yml", want: "docker/compose.yml"},
		{in: "./docker/compose.yaml", want: "docker/compose.yaml"},
		{in: "../compose.yml", wantErr: true},
		{in: "docker/../../compose.yml", wantErr: true},
		{in: "/etc/compose.yml", wantErr: true},
		{in: "docker\\compose.yml", wantErr: true},
		{in: "docker/compose.txt", wantErr: true},
		{in: ".", wantErr: true},
		{in: strings.Repeat("a/", 300) + "c.yml", wantErr: true},
		{in: "docker/com\x07pose.yml", wantErr: true},
		{in: "docker/com\u202epose.yml", wantErr: true},
	}
	for _, tc := range cases {
		got, err := validateComposeRepoPath(tc.in)
		if (err != nil) != tc.wantErr || (!tc.wantErr && got != tc.want) {
			t.Errorf("validateComposeRepoPath(%q) = %q, %v", tc.in, got, err)
		}
	}
}

const composeRepoFile = "services:\n  web:\n    image: nginx:1.27\n  worker:\n    image: busybox:1.36\n"

func newComposeGitRouter(t *testing.T) (*Router, *store.DB, *http.Cookie, *[]fakeGitSourceFetchCall, string) {
	t.Helper()
	rt, db, cookie, _ := newQueueRouter(t)
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docker"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docker", "compose.yml"), []byte(composeRepoFile), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := &[]fakeGitSourceFetchCall{}
	rt.gitSourceFetch = newFakeGitSourceFetch(calls, repo, new(bool), nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/stack/compose", "services:\n  web:\n    image: nginx:1.27\n"))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed compose deploy = %d %s", rec.Code, rec.Body.String())
	}
	return rt, db, cookie, calls, repo
}

func putComposeSource(t *testing.T, rt *Router, cookie *http.Cookie, app, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/"+app+"/compose-source", body))
	return rec
}

// pushCompose sends a push and waits for the compose attempt it started.
func pushCompose(t *testing.T, rt *Router, db *store.DB, gs store.GitSource, payload string) (int, string, store.DeployAttempt) {
	t.Helper()
	status, msg := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, []byte(payload), http.Header{})
	if status != http.StatusAccepted || !strings.HasPrefix(msg, "deploy queued: ") {
		return status, msg, store.DeployAttempt{}
	}
	id := strings.Fields(msg)[2]
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		a, err := db.GetDeployAttempt(context.Background(), id)
		if err == nil && a.Status != store.DeployAttemptStatusRunning && a.Status != store.DeployAttemptStatusQueued {
			return status, msg, *a
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("attempt %s never finished", id)
	return 0, "", store.DeployAttempt{}
}

func connectStack(t *testing.T, rt *Router, db *store.DB, cookie *http.Cookie) store.GitSource {
	t.Helper()
	rec := putComposeSource(t, rt, cookie, "stack", `{"repo_url":"https://github.com/org/stack.git","branch":"main","compose_path":"docker/compose.yml"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"webhook_secret"`) {
		t.Fatalf("PUT compose-source = %d %s", rec.Code, rec.Body.String())
	}
	gs, err := db.GetGitSource(context.Background(), "stack")
	if err != nil || gs.ComposePath != "docker/compose.yml" {
		t.Fatalf("stored source = %+v, %v", gs, err)
	}
	return *gs
}

func TestComposeGitSourcePushDeploysConfiguredPath(t *testing.T) {
	rt, db, cookie, calls, repo := newComposeGitRouter(t)
	ctx := context.Background()
	gs := connectStack(t, rt, db, cookie)

	status, msg := rt.processGitPushWebhookPayload(ctx, "stack", gs, []byte(`{"ref":"refs/heads/dev","before":"aaa","after":"ddd"}`), http.Header{})
	if status != http.StatusOK || !strings.Contains(msg, "ignor") || len(*calls) != 0 {
		t.Fatalf("other branch push = %d %q, fetches %d", status, msg, len(*calls))
	}
	release := gs
	release.TriggerMode = "release"
	if status, msg := rt.processGitPushWebhookPayload(ctx, "stack", release, pushPayload("eee"), http.Header{}); status != http.StatusOK || len(*calls) != 0 {
		t.Fatalf("branch push in release trigger mode = %d %q", status, msg)
	}

	status, msg, a := pushCompose(t, rt, db, gs, string(pushPayload("bbb")))
	if a.Status != store.DeployAttemptStatusSucceeded || a.ServiceName != "stack" || a.CommitSHA != "bbb" {
		t.Fatalf("push = %d %q, attempt %+v", status, msg, a)
	}
	members, _ := db.ListServicesByApp(ctx, "stack")
	if len(members) != 2 {
		t.Fatalf("members = %+v", members)
	}
	rev, err := db.GetComposeRevision(ctx, "stack", 0)
	if err != nil || rev.Source != store.ComposeRevisionSourceGit || rev.CreatedBy != "git:bbb" {
		t.Fatalf("revision = %+v, %v", rev, err)
	}

	fetches := len(*calls)
	if status, msg := rt.processGitPushWebhookPayload(ctx, "stack", gs, pushPayload("bbb"), http.Header{}); status != http.StatusOK || !strings.Contains(msg, "already deployed") || len(*calls) != fetches {
		t.Fatalf("replayed push = %d %q", status, msg)
	}
	if status, msg := rt.processGitPushWebhookPayload(ctx, "stack", gs, []byte(`{"ref":"refs/heads/main","before":"zzz","after":"aaa"}`), http.Header{}); status != http.StatusOK || !strings.Contains(msg, "older") || len(*calls) != fetches {
		t.Fatalf("older push = %d %q", status, msg)
	}
	if _, msg, a := pushCompose(t, rt, db, gs, `{"ref":"refs/heads/main","before":"bbb","after":"ccc"}`); a.Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("newer push = %q, attempt %+v", msg, a)
	}

	outside := filepath.Join(t.TempDir(), "evil.yml")
	if err := os.WriteFile(outside, []byte(composeRepoFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "escape.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.readRepoComposeFile(ctx, repo, "escape.yml"); err == nil {
		t.Fatal("symlink escaping the checkout was read")
	}
	if _, err := rt.readRepoComposeFile(ctx, repo, "../evil.yml"); err == nil {
		t.Fatal("traversal path was read")
	}
}

func TestCheckRepoComposeKeysRefusesOutsideFiles(t *testing.T) {
	for _, body := range []string{
		"services:\n  web:\n    build: .\n",
		"include:\n  - ../other.yml\nservices:\n  web:\n    image: nginx\n",
		"services:\n  web:\n    image: nginx\n    extends:\n      file: ../base.yml\n      service: base\n",
	} {
		if err := checkRepoComposeKeys([]byte(body)); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
	if err := checkRepoComposeKeys([]byte("services:\n  web:\n    image: nginx\n    env_file: .env\n")); err != nil {
		t.Errorf("env_file is ignored like a pasted file, got %v", err)
	}
}

func composePushAt(sha, before string, sec int) []byte {
	return []byte(fmt.Sprintf(`{"ref":"refs/heads/main","before":%q,"after":%q,"head_commit":{"id":%q,"timestamp":"2026-11-09T10:00:%02dZ"}}`, before, sha, sha, sec))
}

func countComposeAttempts(t *testing.T, db *store.DB) map[string]int {
	t.Helper()
	attempts, err := db.ListDeployAttempts(context.Background(), "stack")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, a := range attempts {
		out[a.Status]++
	}
	return out
}

func awaitComposeStatus(t *testing.T, db *store.DB, want map[string]int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got := countComposeAttempts(t, db)
		ok := true
		for k, v := range want {
			if got[k] != v {
				ok = false
			}
		}
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("attempts = %v, want %v", countComposeAttempts(t, db), want)
}

// Twenty rapid pushes behind a running deploy coalesce to one pending, newest wins,
// an out-of-order older delivery is refused, and a manual rollback waits for the lock.
func TestComposeGitSourceDeploysSerializedAndCoalesced(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	gs := connectStack(t, rt, db, cookie)
	mu := composeGitLock("stack")
	mu.Lock()
	locked := true
	defer func() {
		if locked {
			mu.Unlock()
		}
	}()
	reqCtx, cancel := context.WithCancel(context.Background())
	if status, msg := rt.processGitPushWebhookPayload(reqCtx, "stack", gs, composePushAt("c00", "base", 0), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("first push = %d %q", status, msg)
	}
	cancel()
	awaitComposeStatus(t, db, map[string]int{store.DeployAttemptStatusRunning: 1})
	prev := "c00"
	for i := 1; i < 20; i++ {
		sha := fmt.Sprintf("c%02d", i)
		if i == 19 {
			continue
		}
		if status, msg := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, composePushAt(sha, prev, i+1), http.Header{}); status != http.StatusAccepted {
			t.Fatalf("push %s = %d %q", sha, status, msg)
		}
		prev = sha
	}
	// The out-of-order older delivery: committed before the newest pending one.
	if status, msg := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, composePushAt("c19", "c00", 1), http.Header{}); status != http.StatusOK || !strings.Contains(msg, "older") {
		t.Fatalf("older delivery = %d %q", status, msg)
	}
	got := countComposeAttempts(t, db)
	if got[store.DeployAttemptStatusRunning] != 1 || got[store.DeployAttemptStatusQueued] != 1 || got[store.DeployAttemptStatusSuperseded] != 17 {
		t.Fatalf("attempts while locked = %v", got)
	}

	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/stack/compose/redeploy", `{}`))
		done <- rec.Code
	}()
	select {
	case code := <-done:
		t.Fatalf("redeploy ran while a push held the app lock: %d", code)
	case <-time.After(100 * time.Millisecond):
	}
	locked = false
	mu.Unlock()
	if code := <-done; code != http.StatusAccepted {
		t.Fatalf("redeploy after the push = %d", code)
	}
	awaitComposeStatus(t, db, map[string]int{store.DeployAttemptStatusSucceeded: 2, store.DeployAttemptStatusSuperseded: 17})
	cur, err := db.GetDeployCursor(context.Background(), "stack")
	if err != nil || cur.CommitSHA != "c18" {
		t.Fatalf("cursor = %+v, %v (want the newest commit c18)", cur, err)
	}
}

func commitRepoFile(t *testing.T, repo *git.Repository, dir, msg string, when time.Time) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker", "compose.yml"), []byte(composeRepoFile+"# "+msg+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("docker/compose.yml"); err != nil {
		t.Fatal(err)
	}
	sig := &object.Signature{Name: "dev", Email: "dev@example.test", When: when}
	h, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err)
	}
	return h.String()
}

// Deploy-now records an attempt and a cursor carrying the commit's parent and time,
// so a late push of the parent is refused as older.
func TestComposeGitSourceDeployNowRecordsAttemptAndCursor(t *testing.T) {
	rt, db, cookie, _, dir := newComposeGitRouter(t)
	gs := connectStack(t, rt, db, cookie)
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	parent := commitRepoFile(t, repo, dir, "one", time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC))
	head := commitRepoFile(t, repo, dir, "two", time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC))
	rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) { return head, nil }
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/stack/compose-source/deploy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("deploy now = %d %s", rec.Code, rec.Body.String())
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "stack")
	if len(attempts) != 1 || attempts[0].Status != store.DeployAttemptStatusSucceeded || attempts[0].CommitSHA != head || attempts[0].Source != store.DeployAttemptSourceManual {
		t.Fatalf("attempts = %+v", attempts)
	}
	cur, err := db.GetDeployCursor(context.Background(), "stack")
	if err != nil || cur.CommitSHA != head || cur.Before != parent || cur.CommitAt.IsZero() {
		t.Fatalf("cursor = %+v, %v", cur, err)
	}
	late := []byte(fmt.Sprintf(`{"ref":"refs/heads/main","before":"base","after":%q}`, parent))
	if status, msg := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, late, http.Header{}); status != http.StatusOK || !strings.Contains(msg, "older") {
		t.Fatalf("late push of the parent = %d %q", status, msg)
	}
}

// startRunningCompose holds the app lock with one push running, the shape every
// queue test below needs; the returned func releases the lock once.
func startRunningCompose(t *testing.T, rt *Router, db *store.DB, gs store.GitSource) func() {
	t.Helper()
	mu := composeGitLock("stack")
	mu.Lock()
	var once sync.Once
	unlock := func() { once.Do(mu.Unlock) }
	t.Cleanup(unlock)
	if status, msg := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, composePushAt("c00", "base", 0), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("first push = %d %q", status, msg)
	}
	awaitComposeStatus(t, db, map[string]int{store.DeployAttemptStatusRunning: 1})
	return unlock
}

func TestComposeGitSourceOlderEnqueueAfterNewerKeepsNewer(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	gs := connectStack(t, rt, db, cookie)
	unlock := startRunningCompose(t, rt, db, gs)
	if status, msg := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, composePushAt("bbb", "aaa", 20), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("push B = %d %q", status, msg)
	}
	// A CI-gated older commit lands after B through the path that skips admission.
	at := time.Date(2026, 11, 9, 10, 0, 10, 0, time.UTC)
	status, msg := rt.deployFromGitSource(context.Background(), "stack", gs, "aaa", "aaa", &store.DeployOrder{Automatic: true, CommitSHA: "aaa", CommitAt: at})
	if status != http.StatusOK || !strings.Contains(msg, "older than the queued") {
		t.Fatalf("older enqueue = %d %q", status, msg)
	}
	if got := countComposeAttempts(t, db); got[store.DeployAttemptStatusQueued] != 1 || got[store.DeployAttemptStatusSuperseded] != 1 {
		t.Fatalf("attempts = %v", got)
	}
	unlock()
	awaitComposeStatus(t, db, map[string]int{store.DeployAttemptStatusSucceeded: 2})
	if cur, _ := db.GetDeployCursor(context.Background(), "stack"); cur.CommitSHA != "bbb" {
		t.Fatalf("cursor = %+v, want bbb", cur)
	}
}

func TestComposeGitSourceUnorderedEnqueuesKeepBoth(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	gs := connectStack(t, rt, db, cookie)
	startRunningCompose(t, rt, db, gs)
	var wg sync.WaitGroup
	for _, sha := range []string{"u1", "u2"} {
		wg.Add(1)
		go func(sha string) {
			defer wg.Done()
			rt.deployFromGitSource(context.Background(), "stack", gs, sha, sha, &store.DeployOrder{Automatic: true, CommitSHA: sha})
		}(sha)
	}
	wg.Wait()
	if got := countComposeAttempts(t, db); got[store.DeployAttemptStatusQueued] != 2 || got[store.DeployAttemptStatusSuperseded] != 0 {
		t.Fatalf("attempts = %v", got)
	}
}

func TestComposeGitSourceQueuedHeldByMemberFreeze(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	ctx := context.Background()
	gs := connectStack(t, rt, db, cookie)
	startRunningCompose(t, rt, db, gs)
	if status, msg := rt.processGitPushWebhookPayload(ctx, "stack", gs, composePushAt("c01", "c00", 1), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("push = %d %q", status, msg)
	}
	queued, _ := db.ListQueuedDeployAttempts(ctx)
	if len(queued) != 1 || rt.queuedFrozen(ctx, queued[0]) {
		t.Fatalf("queued = %+v before the freeze", queued)
	}
	if _, err := db.ReplaceDeployFreezeWindows(ctx, "app:stack-web", []store.DeployFreezeWindow{{Cron: "* * * * *", Duration: time.Hour, Timezone: "UTC", Reason: "test"}}); err != nil {
		t.Fatal(err)
	}
	if !rt.queuedFrozen(ctx, queued[0]) {
		t.Fatal("a member freeze did not hold the queued compose deploy")
	}
}

func TestComposeGitSourceReleaseReplayRunsInHeldAttempt(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	ctx := context.Background()
	connectStack(t, rt, db, cookie)
	id, ok := rt.saveComposeAttempt(ctx, "stack", "bbb", store.DeployAttemptSourceWebhook)
	if !ok {
		t.Fatal("save attempt")
	}
	a, _ := db.GetDeployAttempt(ctx, id)
	hr := deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: "bbb", CommitLabel: "bbb", CommitSHA: "bbb"}
	if err := rt.ReleaseHandlers()[deploy.HeldKindGit](ctx, *a, hr); err != nil {
		t.Fatal(err)
	}
	members, _ := db.ListServicesByApp(ctx, "stack")
	if got := countComposeAttempts(t, db); len(members) != 2 || got[store.DeployAttemptStatusQueued] != 0 || len(got) != 1 {
		t.Fatalf("replay enqueued instead of applying: members %d, attempts %v", len(members), got)
	}
}

func TestComposeDeployWaitsForPushLock(t *testing.T) {
	rt, _, cookie, _, _ := newComposeGitRouter(t)
	mu := composeGitLock("stack")
	mu.Lock()
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/stack/compose", composeRepoFile))
		done <- rec.Code
	}()
	select {
	case code := <-done:
		mu.Unlock()
		t.Fatalf("compose deploy ran while a push held the app lock: %d", code)
	case <-time.After(100 * time.Millisecond):
	}
	mu.Unlock()
	if code := <-done; code != http.StatusOK {
		t.Fatalf("compose deploy after the push = %d", code)
	}
}

func TestComposeGitSourcePushRefusesForeignServiceName(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	ctx := context.Background()
	gs := connectStack(t, rt, db, cookie)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "stack-worker", Image: "busybox:1.36", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	status, msg, _ := rt.applyComposeGitDeploy(ctx, "stack", gs, "bbb", "bbb", nil)
	if status != http.StatusConflict || !strings.Contains(msg, "already exists") {
		t.Fatalf("collision = %d %q", status, msg)
	}
}

func TestComposeGitSourceProtectedEnvironmentHolds(t *testing.T) {
	rt, db, cookie, calls, _ := newComposeGitRouter(t)
	ctx := context.Background()
	gs := connectStack(t, rt, db, cookie)
	if _, _, a := pushCompose(t, rt, db, gs, string(pushPayload("bbb"))); a.Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("first push attempt = %+v", a)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "p1", Name: "p1", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []store.Environment{
		{ID: "env-stage", ProjectID: "p1", Name: "staging"},
		{ID: "env-prod", ProjectID: "p1", Name: "production", Protected: true},
	} {
		e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := db.SaveEnvironment(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetServiceEnvironment(ctx, "stack-web", "env-stage"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "stack-worker", "env-prod"); err != nil {
		t.Fatal(err)
	}
	fetches := len(*calls)
	status, msg := rt.processGitPushWebhookPayload(ctx, "stack", gs, []byte(`{"ref":"refs/heads/main","before":"bbb","after":"ccc"}`), http.Header{})
	if status != http.StatusAccepted || !strings.HasPrefix(msg, "held:") || len(*calls) != fetches {
		t.Fatalf("protected push = %d %q, fetches %d", status, msg, len(*calls))
	}
}

func TestComposeGitSourceRejectsPollTriggerMode(t *testing.T) {
	rt, _, cookie, _, _ := newComposeGitRouter(t)
	rec := putComposeSource(t, rt, cookie, "stack", `{"repo_url":"https://github.com/org/stack.git","trigger_mode":"poll"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "push or release") {
		t.Fatalf("poll trigger mode = %d %s", rec.Code, rec.Body.String())
	}
}

func TestComposeGitSourceMutuallyExclusiveWithServiceSource(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	ctx := context.Background()
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	if rec := putComposeSource(t, rt, cookie, "web", `{"repo_url":"https://github.com/org/web.git"}`); rec.Code != http.StatusConflict {
		t.Fatalf("compose source over a service source = %d %s", rec.Code, rec.Body.String())
	}
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "compose", ComposePath: "compose.yaml"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","build_type":"dockerfile"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("service source over a compose source = %d %s", rec.Code, rec.Body.String())
	}
}

func freezeApp(t *testing.T, db *store.DB, service string) {
	t.Helper()
	if _, err := db.ReplaceDeployFreezeWindows(context.Background(), store.DeployFreezeScopeApp(service), []store.DeployFreezeWindow{{Cron: "* * * * *", Duration: time.Hour, Timezone: "UTC", Reason: "test"}}); err != nil {
		t.Fatal(err)
	}
}

// A released (freeze end) or approved compose replay must not apply while the app or
// any member is frozen; both paths run the HeldKindGit release handler.
func TestComposeGitSourceReplayRefusedDuringFreeze(t *testing.T) {
	for _, frozen := range []string{"stack-web", "stack"} {
		t.Run(frozen, func(t *testing.T) {
			rt, db, cookie, _, _ := newComposeGitRouter(t)
			ctx := context.Background()
			connectStack(t, rt, db, cookie)
			freezeApp(t, db, frozen)
			id, _ := rt.saveComposeAttempt(ctx, "stack", "bbb", store.DeployAttemptSourceWebhook)
			a, _ := db.GetDeployAttempt(ctx, id)
			hr := deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: "bbb", CommitLabel: "bbb", CommitSHA: "bbb"}
			err := rt.ReleaseHandlers()[deploy.HeldKindGit](ctx, *a, hr)
			members, _ := db.ListServicesByApp(ctx, "stack")
			if err == nil || !strings.Contains(err.Error(), "not deployed") || len(members) != 1 {
				t.Fatalf("replay during a freeze of %s = %v, members %d", frozen, err, len(members))
			}
		})
	}
}

func TestComposeDownCancelsWaitingAndRefusesReplay(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	gs := connectStack(t, rt, db, cookie)
	octx, _ := withWebhookOutcome(context.Background())
	if status, msg := rt.processGitPushWebhookPayload(octx, "stack", gs, composePushAt("c01", "c00", 1), http.Header{}); status != http.StatusAccepted {
		t.Fatalf("push = %d %q", status, msg)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/stack/compose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("compose down = %d %s", rec.Code, rec.Body.String())
	}
	if got := countComposeAttempts(t, db); got[store.DeployAttemptStatusCanceled] != 1 || got[store.DeployAttemptStatusQueued] != 0 {
		t.Fatalf("attempts after down = %v", got)
	}
	ctx := withAdoptedAttempt(context.Background(), "replayed")
	status, msg := rt.deployFromGitSource(ctx, "stack", gs, "c02", "c02", nil)
	members, _ := db.ListServicesByApp(context.Background(), "stack")
	if status != http.StatusGone || len(members) != 0 {
		t.Fatalf("replay after down = %d %q, members %d", status, msg, len(members))
	}
	if status, _ := rt.processGitPushWebhookPayload(context.Background(), "stack", gs, composePushAt("c03", "c02", 3), http.Header{}); status != http.StatusNotFound {
		t.Fatalf("push after down = %d, want 404 (app not found)", status)
	}
}

type failingFreezeStore struct {
	DeploySafetyStore
	scope string
}

func (f failingFreezeStore) ListDeployFreezeWindows(ctx context.Context, scopes ...string) ([]store.DeployFreezeWindow, error) {
	for _, s := range scopes {
		if s == f.scope {
			return nil, errors.New("freeze store unavailable")
		}
	}
	return f.DeploySafetyStore.ListDeployFreezeWindows(ctx, scopes...)
}

type failingGroups struct{ AppGroupLister }

func (failingGroups) ListServicesByApp(context.Context, string) ([]store.DesiredService, error) {
	return nil, errors.New("list unavailable")
}

func TestComposeMemberFreezeFailsClosedOnlyForCompose(t *testing.T) {
	rt, db, cookie, _, _ := newComposeGitRouter(t)
	ctx := context.Background()
	connectStack(t, rt, db, cookie)
	orig := rt.deploySafety
	rt.deploySafety = failingFreezeStore{DeploySafetyStore: orig, scope: store.DeployFreezeScopeApp("stack-web")}
	if st := rt.composeMemberFreeze(ctx, "stack", deploy.FreezeStatus{}); !st.Frozen || st.Reason != "freeze check unavailable" {
		t.Fatalf("member freeze error = %+v, want held", st)
	}
	if !rt.queuedFrozen(ctx, store.DeployAttempt{ServiceName: "stack"}) {
		t.Fatal("queued compose deploy ran despite a failed member freeze check")
	}
	rt.deploySafety = orig
	groups := rt.appGroups
	rt.appGroups = failingGroups{groups}
	if st := rt.composeMemberFreeze(ctx, "stack", deploy.FreezeStatus{}); !st.Frozen {
		t.Fatal("unreadable members of a compose app did not hold")
	}
	if st := rt.composeMemberFreeze(ctx, "web", deploy.FreezeStatus{}); st.Frozen {
		t.Fatal("a single-service app was held by a members list error")
	}
}

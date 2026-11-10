// TestGitPush_Live_RealClientThroughAPI pushes a tiny repository with the
// system git client to a real *api.Router over TCP and checks the deploy
// record, the flag gate, token rejection and that no token reaches the
// client output, the API responses or the server log. The builder is a
// recording fake, so this proves the push path, not a real image build.
package e2e

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eGitPushAdminUsername = "e2e-git-push-admin"
	e2eGitPushAdminPassword = "e2e-git-push-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eGitPushApp           = "levelrail-test-e2e-git-push"
)

type gitPushBuilder struct {
	mu    sync.Mutex
	files map[string]string
	calls int
}

func (b *gitPushBuilder) Deploy(_ context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.files = map[string]string{}
	entries, _ := os.ReadDir(req.SourceDir)
	for _, e := range entries {
		if data, err := os.ReadFile(filepath.Join(req.SourceDir, e.Name())); err == nil { //nolint:gosec // test checkout dir
			b.files[e.Name()] = string(data)
		}
	}
	if progress != nil {
		progress(build.ProgressEvent{Log: "fake build output\n", Stream: "stdout"})
	}
	return e2eGitPushApp + ":" + req.CommitSHA, nil
}

func (b *gitPushBuilder) DeploySpec(context.Context, deploy.MultiRequest, func(string, build.ProgressEvent)) ([]deploy.ServiceOutcome, error) {
	return nil, nil
}

type lockedLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func gitPushGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test-only, literal git with test-built args
	cmd.Dir = dir
	// Inherited GIT_* variables would point git at the real repository when run under a hook.
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=dev@example.com")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestGitPush_Live_RealClientThroughAPI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	experimental.Set(experimental.GitPush)
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: e2eGitPushApp, Image: "nginx:git-push", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	logs := &lockedLog{}
	builder := &gitPushBuilder{}
	cache := sourcecache.New(sourcecache.Config{Dir: t.TempDir()}, nil)
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	rt := api.NewRouter(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), b, db, api.WithBuilder(builder), api.WithSourceCache(cache))
	ts := newE2ETestServer(t, rt)
	if err := api.BootstrapAdmin(ctx, db, e2eGitPushAdminUsername, e2eGitPushAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, ts.URL, e2eGitPushAdminUsername, e2eGitPushAdminPassword)

	remoteAPI := ts.URL + "/api/v1/apps/" + e2eGitPushApp + "/git-remote"
	status, body := postJSON(t, admin, remoteAPI, "")
	if status != http.StatusOK {
		t.Fatalf("issue remote: status = %d, body = %s", status, body)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &issued); err != nil || issued.Token == "" {
		t.Fatalf("issue remote: decode %s: %v", body, err)
	}
	// The stored remote must never hand the token back after issue.
	if status, got := requestJSON(t, admin, http.MethodGet, remoteAPI, ""); status != http.StatusOK || strings.Contains(string(got), issued.Token) {
		t.Fatalf("get remote: status = %d, token echoed = %t", status, strings.Contains(string(got), issued.Token))
	}

	withToken := func(token string) string {
		u, err := url.Parse(ts.URL + "/api/v1/git/" + e2eGitPushApp + ".git")
		if err != nil {
			t.Fatalf("parse remote url: %v", err)
		}
		u.User = url.UserPassword("git", token)
		return u.String()
	}

	dir := t.TempDir()
	if out, err := gitPushGit(t, dir, "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("e2e"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "first"}} {
		if out, err := gitPushGit(t, dir, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	sha, err := gitPushGit(t, dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v %s", err, sha)
	}
	sha = strings.TrimSpace(sha)

	// A wrong token is rejected before anything is built.
	if out, err := gitPushGit(t, dir, "push", withToken("gp_wrong"), "main"); err == nil || !strings.Contains(out, "Authentication failed") {
		t.Fatalf("wrong token: err=%v\n%s", err, out)
	}
	if builder.calls != 0 {
		t.Fatalf("builder ran %d times for a rejected push", builder.calls)
	}

	out, err := gitPushGit(t, dir, "push", withToken(issued.Token), "main")
	if err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Deploy succeeded") || strings.Contains(out, issued.Token) {
		t.Fatalf("push output lacks success or echoes the token:\n%s", out)
	}
	if builder.calls != 1 || builder.files["hello.txt"] != "e2e" {
		t.Fatalf("builder calls=%d files=%v", builder.calls, builder.files)
	}

	attempts, err := db.ListDeployAttempts(ctx, e2eGitPushApp)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v err=%v", attempts, err)
	}
	if a := attempts[0]; a.Source != store.DeployAttemptSourceGitPush || a.CommitSHA != sha || a.Status != store.DeployAttemptStatusSucceeded {
		t.Fatalf("attempt = %+v, want git_push succeeded at %s", a, sha)
	}
	status, historyRaw := requestJSON(t, admin, http.MethodGet, ts.URL+"/api/v1/apps/"+e2eGitPushApp+"/deploy-attempts", "")
	history := string(historyRaw)
	if status != http.StatusOK || !strings.Contains(history, sha) || strings.Contains(history, issued.Token) {
		t.Fatalf("deploy history: status = %d, has sha = %t, token echoed = %t", status, strings.Contains(history, sha), strings.Contains(history, issued.Token))
	}

	// Flag off: both the management route and the git endpoint are gated, and a push builds nothing.
	experimental.Set()
	status, body = postJSON(t, admin, remoteAPI, "")
	if status != http.StatusNotFound || !strings.Contains(body, api.ExperimentalDisabledCode) {
		t.Fatalf("flag off, issue remote: status = %d, body = %s", status, body)
	}
	if out, err := gitPushGit(t, dir, "commit", "-q", "--allow-empty", "-m", "second"); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	if out, err := gitPushGit(t, dir, "push", withToken(issued.Token), "main"); err == nil {
		t.Fatalf("push with the flag off succeeded:\n%s", out)
	}
	if builder.calls != 1 {
		t.Fatalf("builder ran %d times, want 1 (flag off must not build)", builder.calls)
	}

	if strings.Contains(logs.String(), issued.Token) {
		t.Fatal("server log contains the push token")
	}
}

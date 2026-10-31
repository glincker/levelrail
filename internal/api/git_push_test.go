package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

type pushTestBuilder struct {
	*fakeBuilder
	mu    sync.Mutex
	fail  error
	files map[string]string
	calls int
}

func (b *pushTestBuilder) Deploy(_ context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error) {
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
		progress(build.ProgressEvent{Log: "step one of the build\n", Stream: "stdout"})
	}
	if b.fail != nil {
		return "", b.fail
	}
	return "web:" + req.CommitSHA, nil
}

func (b *pushTestBuilder) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

type pushEnv struct {
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
	srv    *httptest.Server
	b      *pushTestBuilder
}

func newPushEnv(t *testing.T) *pushEnv {
	t.Helper()
	enableAllExperimental()
	db := openTestDB(t)
	b := &pushTestBuilder{fakeBuilder: newFakeBuilder("web:x", nil)}
	cache := sourcecache.New(sourcecache.Config{Dir: t.TempDir()}, nil)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(b), WithSourceCache(cache))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedApp(t, db, "api")
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)
	return &pushEnv{rt: rt, db: db, cookie: cookie, srv: srv, b: b}
}

func newPushEnvFor(t *testing.T, rt *Router, db *store.DB, cookie *http.Cookie, b *pushTestBuilder) *pushEnv {
	t.Helper()
	srv := httptest.NewServer(rt.Handler())
	t.Cleanup(srv.Close)
	return &pushEnv{rt: rt, db: db, cookie: cookie, srv: srv, b: b}
}

func (e *pushEnv) issue(t *testing.T, app string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, http.MethodPost, "/api/v1/apps/"+app+"/git-remote", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("issue token status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token      string `json:"token"`
		AddCommand string `json:"add_command"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.Token, gitPushTokenPrefix) || !strings.Contains(out.AddCommand, "git remote add ") {
		t.Fatalf("unexpected issue response %+v", out)
	}
	return out.Token
}

func (e *pushEnv) remoteURL(app string) string { return e.srv.URL + "/api/v1/git/" + app + ".git" }

func pushWorkRepo(t *testing.T, files map[string]string) (*git.Repository, plumbing.Hash) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	return repo, pushCommit(t, repo, dir, files, "first")
}

func pushCommit(t *testing.T, repo *git.Repository, dir string, files map[string]string, msg string) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	h, err := wt.Commit(msg, &git.CommitOptions{Author: &object.Signature{Name: "Dev", Email: "dev@example.com", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func pushTo(repo *git.Repository, url, token, refspec string) error {
	_ = repo.DeleteRemote("lr")
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "lr", URLs: []string{url}}); err != nil {
		return err
	}
	return repo.Push(&git.PushOptions{
		RemoteName: "lr",
		RefSpecs:   []config.RefSpec{config.RefSpec(refspec)},
		Auth:       &githttp.BasicAuth{Username: "git", Password: token},
	})
}

func worktreeDir(t *testing.T, repo *git.Repository) string {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	return wt.Filesystem.Root()
}

func advertisedRefs(t *testing.T, e *pushEnv, app, token string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.remoteURL(app)+"/info/refs?service=git-receive-pack", nil)
	req.SetBasicAuth("git", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("info/refs status = %d: %s", resp.StatusCode, body)
	}
	return string(body)
}

func TestGitPush_DeploysPushedCommit(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	repo, first := pushWorkRepo(t, map[string]string{"hello.txt": "one"})
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("first push: %v", err)
	}
	if e.b.callCount() != 1 || e.b.files["hello.txt"] != "one" {
		t.Fatalf("builder saw calls=%d files=%v", e.b.callCount(), e.b.files)
	}
	attempts, err := e.db.ListDeployAttempts(context.Background(), "web")
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v err=%v", attempts, err)
	}
	if a := attempts[0]; a.Source != store.DeployAttemptSourceGitPush || a.CommitSHA != first.String() || a.Status != store.DeployAttemptStatusSucceeded || a.Branch != "main" {
		t.Fatalf("attempt = %+v", a)
	}
	if !strings.Contains(advertisedRefs(t, e, "web", token), "refs/heads/main") {
		t.Fatal("deployed branch should be advertised for the next push")
	}

	second := pushCommit(t, repo, worktreeDir(t, repo), map[string]string{"hello.txt": "two"}, "second")
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("second push: %v", err)
	}
	if e.b.callCount() != 2 || e.b.files["hello.txt"] != "two" {
		t.Fatalf("second deploy: calls=%d files=%v", e.b.callCount(), e.b.files)
	}
	if got := advertisedRefs(t, e, "web", token); !strings.Contains(got, second.String()) {
		t.Fatalf("advertisement does not carry the new head: %q", got)
	}
}

func TestGitPush_AuthFailures(t *testing.T) {
	e := newPushEnv(t)
	webToken := e.issue(t, "web")
	apiToken := e.issue(t, "api")
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "a"})

	if err := pushTo(repo, e.remoteURL("web"), "gp_wrong", "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("wrong token pushed")
	}
	if err := pushTo(repo, e.remoteURL("web"), apiToken, "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("another app's token pushed")
	}
	if err := pushTo(repo, e.remoteURL("ghost"), webToken, "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("token pushed to an app that does not exist")
	}
	if e.b.callCount() != 0 {
		t.Fatalf("builder ran %d times for rejected pushes", e.b.callCount())
	}

	resp, err := http.Get(e.remoteURL("web") + "/info/refs?service=git-receive-pack") //nolint:noctx // test against a local server
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Basic") {
		t.Fatalf("anonymous = %d %q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

func TestGitPush_BadTokenBacksOff(t *testing.T) {
	e := newPushEnv(t)
	e.issue(t, "web")
	var last int
	for range 8 {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.remoteURL("web")+"/info/refs?service=git-receive-pack", nil)
		req.SetBasicAuth("git", "gp_guess")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("repeated bad tokens ended with %d, want 429", last)
	}
}

func TestGitPush_RateLimitsRequests(t *testing.T) {
	t.Setenv(envGitPushRatePerMinute, "3")
	e := newPushEnv(t)
	token := e.issue(t, "web")
	got429 := false
	for range 8 {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.remoteURL("web")+"/info/refs?service=git-receive-pack", nil)
		req.SetBasicAuth("git", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
		}
	}
	if !got429 {
		t.Fatal("no request was rate limited")
	}
}

func TestGitPush_RotationInvalidatesOldToken(t *testing.T) {
	e := newPushEnv(t)
	old := e.issue(t, "web")
	fresh := e.issue(t, "web")
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "a"})
	if err := pushTo(repo, e.remoteURL("web"), old, "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("rotated token still pushes")
	}
	if err := pushTo(repo, e.remoteURL("web"), fresh, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("new token: %v", err)
	}
}

func TestGitPush_RevokeStopsPushes(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, http.MethodDelete, "/api/v1/apps/web/git-remote", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d", rec.Code)
	}
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "a"})
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("revoked token pushed")
	}
}

func TestGitPush_OtherBranchRejected(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "a"})
	err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/dev")
	if err == nil || !strings.Contains(err.Error(), "branch not allowed") {
		t.Fatalf("err = %v, want branch not allowed", err)
	}
	if e.b.callCount() != 0 {
		t.Fatal("a non-deploy branch built")
	}
}

func TestGitPush_ConfiguredBranchDeploys(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, http.MethodPut, "/api/v1/apps/web/git-remote", `{"branch":"release"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("set branch status = %d %s", rec.Code, rec.Body.String())
	}
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "a"})
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("main should no longer deploy")
	}
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/release"); err != nil {
		t.Fatalf("release push: %v", err)
	}
}

func TestGitPush_OversizedPushRejected(t *testing.T) {
	t.Setenv(envGitPushMaxBytes, "4096")
	e := newPushEnv(t)
	token := e.issue(t, "web")
	blob := make([]byte, 64<<10)
	if _, err := rand.Read(blob); err != nil {
		t.Fatal(err)
	}
	repo, _ := pushWorkRepo(t, map[string]string{"big.bin": string(blob)})
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err == nil {
		t.Fatal("oversized push accepted")
	}
	if e.b.callCount() != 0 {
		t.Fatal("oversized push built")
	}
}

func TestGitPush_FailedBuildKeepsPreviousRelease(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	e.b.fail = errors.New("npm ci exited 1")
	repo, _ := pushWorkRepo(t, map[string]string{"a.txt": "a"})
	err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main")
	if err == nil || !strings.Contains(err.Error(), "deploy failed") {
		t.Fatalf("err = %v, want deploy failed", err)
	}
	svc, gerr := e.db.GetDesiredService(context.Background(), "web")
	if gerr != nil || svc.Image != "img:v1" {
		t.Fatalf("desired image changed: %+v err=%v", svc, gerr)
	}
	if strings.Contains(advertisedRefs(t, e, "web", token), "refs/heads/main") {
		t.Fatal("failed deploy advanced the branch, a retry would be skipped")
	}
	e.b.fail = nil
	if err := pushTo(repo, e.remoteURL("web"), token, "refs/heads/main:refs/heads/main"); err != nil {
		t.Fatalf("retry after fixing the build: %v", err)
	}
}

func TestGitPush_FeatureOffAnswers404(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	experimental.Set()
	t.Cleanup(enableAllExperimental)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.remoteURL("web")+"/info/refs?service=git-receive-pack", nil)
	req.SetBasicAuth("git", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGitPush_CloneIsRefused(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.remoteURL("web")+"/info/refs?service=git-upload-pack", nil)
	req.SetBasicAuth("git", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("clone status = %d, want 403", resp.StatusCode)
	}
}

func TestGitPush_TokenIsNeverStoredOrReshown(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	rm, err := e.db.GetGitPushRemote(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rm.TokenHash, token) || len(rm.TokenHash) != 64 {
		t.Fatalf("stored value is not a sha256 digest: %q", rm.TokenHash)
	}
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, http.MethodGet, "/api/v1/apps/web/git-remote", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), rm.TokenHash) {
		t.Fatalf("GET leaked credentials: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGitPush_BranchNameValidation(t *testing.T) {
	for in, want := range map[string]bool{
		"main": true, "release/1.2": true, "": false, "-x": false, "refs/heads/x": false, "a..b": false, "a b": false,
	} {
		if got := validGitPushBranch(in); got != want {
			t.Errorf("validGitPushBranch(%q) = %v, want %v", in, got, want)
		}
	}
}

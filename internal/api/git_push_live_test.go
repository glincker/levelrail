package api

import (
	"crypto/rand"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runRealGit drives the system git client, so the wire protocol is checked
// against the real implementation, including its large-push probe request.
func runRealGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test-only, literal git with test-built args
	cmd.Dir = dir
	cmd.Env = append(cleanGitEnv(),
		"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.com",
		"GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=dev@example.com",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// cleanGitEnv drops inherited GIT_* variables: under a git hook GIT_DIR points at the real repository.
func cleanGitEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			env = append(env, e)
		}
	}
	return env
}

func realGitRepo(t *testing.T, files map[string][]byte) string {
	t.Helper()
	if testing.Short() {
		t.Skip("drives the system git client, runs outside the short lane")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	if out, err := runRealGit(t, dir, "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := runRealGit(t, dir, "add", "-A"); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	if out, err := runRealGit(t, dir, "commit", "-q", "-m", "first"); err != nil {
		t.Fatalf("git commit: %v %s", err, out)
	}
	return dir
}

func authedRemote(t *testing.T, e *pushEnv, token string) string {
	t.Helper()
	u, err := url.Parse(e.remoteURL("web"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("git", token)
	return u.String()
}

func TestGitPushLive_RealClientDeploysAndShowsRemoteLines(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	dir := realGitRepo(t, map[string][]byte{"hello.txt": []byte("live")})

	out, err := runRealGit(t, dir, "push", authedRemote(t, e, token), "main")
	if err != nil {
		t.Fatalf("git push: %v\n%s", err, out)
	}
	for _, want := range []string{"remote: Received commit", "remote: step one of the build", "remote: Deploy succeeded"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, token) {
		t.Errorf("token echoed to the client:\n%s", out)
	}
	if e.b.files["hello.txt"] != "live" {
		t.Fatalf("builder files = %v", e.b.files)
	}

	if out, err := runRealGit(t, dir, "commit", "-q", "--allow-empty", "-m", "second"); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	if out, err := runRealGit(t, dir, "push", authedRemote(t, e, token), "main"); err != nil {
		t.Fatalf("second push (incremental): %v\n%s", err, out)
	}
	if e.b.callCount() != 2 {
		t.Fatalf("deploys = %d, want 2", e.b.callCount())
	}
}

func TestGitPushLive_RealClientSeesBuildFailureAndTokenErrors(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	e.b.fail = errors.New("go build: undefined: widget")
	dir := realGitRepo(t, map[string][]byte{"main.go": []byte("package main")})

	out, err := runRealGit(t, dir, "push", authedRemote(t, e, token), "main")
	if err == nil {
		t.Fatalf("push with a failing build succeeded:\n%s", out)
	}
	for _, want := range []string{"remote: Deploy failed: go build: undefined: widget", "previous release is still running", "[remote rejected]"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}

	out, err = runRealGit(t, dir, "push", authedRemote(t, e, "gp_wrong"), "main")
	if err == nil || !strings.Contains(out, "Authentication failed") {
		t.Fatalf("wrong token: err=%v\n%s", err, out)
	}

	out, err = runRealGit(t, dir, "push", authedRemote(t, e, token), "main:dev")
	if err == nil || !strings.Contains(out, "this app deploys \"main\"") {
		t.Fatalf("branch rule: err=%v\n%s", err, out)
	}
}

func TestGitPushLive_RealClientLargePush(t *testing.T) {
	e := newPushEnv(t)
	token := e.issue(t, "web")
	blob := make([]byte, 3<<20)
	if _, err := rand.Read(blob); err != nil {
		t.Fatal(err)
	}
	dir := realGitRepo(t, map[string][]byte{"blob.bin": blob})
	out, err := runRealGit(t, dir, "push", authedRemote(t, e, token), "main")
	if err != nil {
		t.Fatalf("large push (chunked, probe request): %v\n%s", err, out)
	}
	if e.b.callCount() != 1 {
		t.Fatalf("deploys = %d", e.b.callCount())
	}
}

func TestRunRealGit_IgnoresInheritedGitDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	hookRepo := t.TempDir()
	t.Setenv("GIT_DIR", filepath.Join(hookRepo, ".git"))
	t.Setenv("GIT_WORK_TREE", hookRepo)
	dir := t.TempDir()
	if out, err := runRealGit(t, dir, "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("init did not create .git in the test directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hookRepo, ".git")); err == nil {
		t.Fatal("git wrote into the inherited GIT_DIR")
	}
}

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // test-only git calls
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", name)
	gitT(t, dir, "commit", "-q", "-m", name)
}

// pushCase builds a clone on branch "feat" with a bare origin, then lets each case mutate the pair.
func pushCase(t *testing.T, setup func(t *testing.T, local, bare string)) (string, upInference) {
	t.Helper()
	base := t.TempDir()
	bare := filepath.Join(base, "origin.git")
	local := filepath.Join(base, "shop")
	gitT(t, base, "init", "-q", "--bare", bare)
	gitT(t, base, "init", "-q", "-b", "feat", local)
	gitT(t, local, "remote", "add", "origin", bare)
	commitFile(t, local, "a.txt")
	setup(t, local, bare)
	g := detectGitAt(local)
	inf := upInference{Git: g.detectedGit, Sources: map[string]string{"repo": srcGit}}
	inf.Flags.repo, inf.Flags.ref = g.RepoURL, g.Ref
	return local, inf
}

func TestCheckPushed(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, local, bare string)
		wantProblem string
		wantWarning string
	}{
		{"pushed at HEAD", func(t *testing.T, l, _ string) { gitT(t, l, "push", "-q", "origin", "feat") }, "", ""},
		{"branch missing on remote", func(*testing.T, string, string) {}, "git push -u origin feat", ""},
		{"local ahead of remote", func(t *testing.T, l, _ string) {
			gitT(t, l, "push", "-q", "origin", "feat")
			commitFile(t, l, "b.txt")
		}, "does not include your latest commit", ""},
		{"HEAD is an ancestor of remote", func(t *testing.T, l, _ string) {
			commitFile(t, l, "b.txt")
			gitT(t, l, "push", "-q", "origin", "feat")
			gitT(t, l, "reset", "-q", "--hard", "HEAD~1")
		}, "", ""},
		{"detached HEAD", func(t *testing.T, l, _ string) { gitT(t, l, "checkout", "-q", "--detach") }, "", "detached"},
		{"remote unreachable", func(t *testing.T, l, _ string) {
			gitT(t, l, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
		}, "", "could not reach"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, inf := pushCase(t, tc.setup)
			got := checkPushed(context.Background(), dir, inf, false)
			if tc.wantProblem == "" && got.Problem != "" || tc.wantProblem != "" && !strings.Contains(got.Problem, tc.wantProblem) {
				t.Fatalf("problem = %q, want containing %q", got.Problem, tc.wantProblem)
			}
			if tc.wantWarning == "" && got.Warning != "" || tc.wantWarning != "" && !strings.Contains(got.Warning, tc.wantWarning) {
				t.Fatalf("warning = %q, want containing %q", got.Warning, tc.wantWarning)
			}
		})
	}
}

func TestUpRefusesUnpushedBranch(t *testing.T) {
	f, srv := newFakeUp(t)
	dir, _ := pushCase(t, func(*testing.T, string, string) {})
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"express":"4"},"scripts":{"start":"node ."}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := runUpCLI(t, srv, dir)
	if code == exitOK || !strings.Contains(errOut, "git push -u origin feat") {
		t.Fatalf("code=%d stderr=%q, want non-zero with the push command", code, errOut)
	}
	if f.calls["POST /api/v1/apps"] != 0 {
		t.Fatal("up created the app before the push check")
	}

	if code, _, errOut = runUpCLI(t, srv, dir, "--allow-unpushed"); code != exitOK {
		t.Fatalf("--allow-unpushed code=%d stderr=%q", code, errOut)
	}
}

func TestCheckPushedRejectsHostileOperands(t *testing.T) {
	dir, inf := pushCase(t, func(*testing.T, string, string) {})
	marker := filepath.Join(t.TempDir(), "pwned")
	tests := []struct{ name, remote, branch string }{
		{"upload-pack remote", "--upload-pack=touch " + marker, "feat"},
		{"config remote", "-c core.sshCommand=touch " + marker, "feat"},
		{"ext transport", "ext::sh -c touch% " + marker, "feat"},
		{"option branch", inf.Flags.repo, "--upload-pack=touch " + marker},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := inf
			in.Flags.repo, in.Flags.ref = tc.remote, tc.branch
			got := checkPushed(context.Background(), dir, in, true)
			if got.Problem == "" && got.Warning == "" {
				t.Fatal("hostile operand passed silently")
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("hostile operand executed a command")
			}
		})
	}
}

func TestSafeTextStripsControls(t *testing.T) {
	if got := safeText("a\x1b[31mb\nc"); strings.ContainsAny(got, "\x1b\n") {
		t.Fatalf("safeText kept controls: %q", got)
	}
}

func TestCheckPushedIgnoresHostileRepoConfig(t *testing.T) {
	t.Setenv(envPushCheckTimeoutSec, "5")
	dir, inf := pushCase(t, func(*testing.T, string, string) {})
	marker := filepath.Join(t.TempDir(), "pwned")
	script := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o700); err != nil { //nolint:gosec // test hook must be executable
		t.Fatal(err)
	}
	authSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="x"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(authSrv.Close)
	gitT(t, dir, "config", "http.sslVerify", "false")
	tests := []struct{ name, key, value, remote string }{
		{"git proxy", "core.gitProxy", script, "git://127.0.0.1:1/shop.git"},
		{"credential helper", "credential.helper", "!" + script, authSrv.URL + "/shop.git"},
		{"askpass", "core.askPass", script, authSrv.URL + "/shop.git"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(marker)
			gitT(t, dir, "config", tc.key, tc.value)
			t.Cleanup(func() { gitT(t, dir, "config", "--unset-all", tc.key) })
			in := inf
			in.Flags.repo = tc.remote
			got := checkPushed(context.Background(), dir, in, true)
			if _, err := os.Stat(marker); err == nil {
				t.Fatalf("%s in .git/config ran a command (check = %+v)", tc.key, got)
			}
		})
	}
}

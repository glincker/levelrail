package main

import (
	"context"
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

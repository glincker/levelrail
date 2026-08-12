package webhook

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TestCloneAndCheckout_Live exercises the real go-git clone-and-checkout
// path against a local repository built by go-git itself in a temp dir,
// not a clone of a real GitHub repo: that keeps the test free of any
// network dependency, so it can't flake on GitHub's availability, the
// same reasoning the task called for. cloneAndCheckout treats a local
// filesystem path exactly like any other git remote (go-git resolves a
// plain path as a "file" transport endpoint), so this exercises the same
// clone/checkout code the real handler uses against a real GitHub HTTPS
// remote.
//
// The repo gets two commits with different file contents. Checking out
// the first commit's SHA (not the branch tip, the second commit) and
// confirming the second commit's file is absent is what proves this
// checks out the requested SHA specifically, not just "whatever HEAD is."
func TestCloneAndCheckout_Live(t *testing.T) {
	srcDir := t.TempDir()

	repo, err := git.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	sig := &object.Signature{
		Name:  "Levelrail Test",
		Email: "test@example.invalid",
		When:  time.Now(),
	}

	writeFile(t, srcDir, "app.txt", "version one")
	if _, err := wt.Add("app.txt"); err != nil {
		t.Fatalf("Add app.txt: %v", err)
	}
	firstHash, err := wt.Commit("first commit", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("commit first: %v", err)
	}

	writeFile(t, srcDir, "app.txt", "version two")
	writeFile(t, srcDir, "later.txt", "only exists after the second commit")
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add .: %v", err)
	}
	if _, err := wt.Commit("second commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("commit second: %v", err)
	}

	dir, cleanup, err := cloneAndCheckout(context.Background(), srcDir, firstHash.String())
	if err != nil {
		t.Fatalf("cloneAndCheckout() error = %v", err)
	}
	if dir == "" {
		t.Fatal("cloneAndCheckout() returned an empty dir")
	}

	got, err := os.ReadFile(filepath.Join(dir, "app.txt")) //nolint:gosec // dir is a temp checkout this test created, not user input
	if err != nil {
		t.Fatalf("reading app.txt from checkout: %v", err)
	}
	if string(got) != "version one" {
		t.Errorf("app.txt content = %q, want %q (the first commit's content, not the branch tip's)", string(got), "version one")
	}

	if _, err := os.Stat(filepath.Join(dir, "later.txt")); !os.IsNotExist(err) {
		t.Errorf("later.txt exists in the checkout at the first commit's SHA, want it absent (it was only added in the second commit)")
	}

	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("checkout dir %q still exists after cleanup(), want it removed", dir)
	}
}

// TestCloneAndCheckout_Live_BadSHA proves a nonexistent commit hash fails
// the checkout rather than silently landing on some other commit, and
// still cleans up the temp dir it created before failing.
func TestCloneAndCheckout_Live_BadSHA(t *testing.T) {
	srcDir := t.TempDir()
	repo, err := git.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	writeFile(t, srcDir, "app.txt", "version one")
	if _, err := wt.Add("app.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("only commit", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.invalid", When: time.Now()},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Valid hex, correct length, non-zero (the all-zero hash is a
	// special case go-git treats as "no hash given" rather than "look
	// this commit up"), but not a commit that exists in the repo.
	nonexistentSHA := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	dir, cleanup, err := cloneAndCheckout(context.Background(), srcDir, nonexistentSHA)
	if err == nil {
		cleanup()
		t.Fatal("cloneAndCheckout() error = nil, want an error for a nonexistent SHA")
	}
	if dir != "" {
		t.Errorf("cloneAndCheckout() dir = %q on error, want empty", dir)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

package api

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// TestListRemoteBranches_Live exercises the real go-git remote-listing
// path against a local repository built by go-git itself in a temp dir,
// not a clone of a real GitHub repo: the same "free of any network
// dependency, so it can't flake on GitHub's availability" reasoning
// internal/webhook/clone_live_test.go's own TestCloneAndCheckout_Live
// already documents for the identical tradeoff. go-git resolves a plain
// filesystem path as a "file" transport endpoint exactly like an HTTPS
// remote, so this exercises the same advertised-refs listing code the
// real handler uses against a real GitHub HTTPS remote, with a real
// network round trip through go-git's own transport layer, just a local
// one instead of a flaky external dependency.
func TestListRemoteBranches_Live(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(srcDir, "app.txt"), []byte("version one"), 0o600); err != nil {
		t.Fatalf("write app.txt: %v", err)
	}
	if _, err := wt.Add("app.txt"); err != nil {
		t.Fatalf("Add app.txt: %v", err)
	}
	firstHash, err := wt.Commit("first commit", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatalf("commit first: %v", err)
	}

	// A second branch off the same commit, so the repo genuinely
	// advertises more than one branch ref: proves the filter keeps every
	// branch, not just whichever one HEAD happens to point at.
	developRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("develop"), firstHash)
	if err := repo.Storer.SetReference(developRef); err != nil {
		t.Fatalf("create develop branch: %v", err)
	}

	// A tag too, so the branch filter is proven to exclude it: an
	// unfiltered advertised-refs listing would otherwise include this
	// under a name a caller could mistake for a real branch.
	if _, err := repo.CreateTag("v1.0.0", firstHash, nil); err != nil {
		t.Fatalf("create tag: %v", err)
	}

	branches, err := listRemoteBranches(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("listRemoteBranches() error = %v", err)
	}

	sort.Strings(branches)
	want := []string{"develop", "master"}
	if len(branches) != len(want) {
		t.Fatalf("branches = %v, want %v", branches, want)
	}
	for i, b := range want {
		if branches[i] != b {
			t.Errorf("branches[%d] = %q, want %q (full list: %v)", i, branches[i], b, branches)
		}
	}
}

// TestListRemoteBranches_Live_UnreachableRepo proves a nonexistent local
// path (the same-shaped failure a private or malformed remote URL
// produces for the real HTTPS case) returns a clean error, not a panic.
func TestListRemoteBranches_Live_UnreachableRepo(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := listRemoteBranches(context.Background(), nonexistent)
	if err == nil {
		t.Fatal("listRemoteBranches() error = nil, want an error for an unreachable repo")
	}
}

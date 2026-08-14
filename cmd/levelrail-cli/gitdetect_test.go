package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestDetectLocalGit_NotARepo(t *testing.T) {
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()

	got := detectLocalGit()
	if got != (detectedGit{}) {
		t.Errorf("detectLocalGit() = %+v, want zero value outside a git repo", got)
	}
}

func TestDetectLocalGit_WithOriginAndBranch(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://example.com/org/repo.git"},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}

	// A commit is required before HEAD resolves to anything: an empty
	// repo's HEAD is a symbolic ref to a branch with no commit yet.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add("f.txt"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// Create and switch to "develop" from the commit just made, whatever
	// go-git's default initial branch name is: this only needs *a*
	// current branch with a real name to detect, not to know that
	// default in advance.
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("develop"), Create: true}); err != nil {
		t.Fatalf("Checkout(create develop): %v", err)
	}

	restore := chdir(t, dir)
	defer restore()

	got := detectLocalGit()
	if got.RepoURL != "https://example.com/org/repo.git" {
		t.Errorf("RepoURL = %q, want %q", got.RepoURL, "https://example.com/org/repo.git")
	}
	if got.Ref != "develop" {
		t.Errorf("Ref = %q, want %q", got.Ref, "develop")
	}
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	return func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore Chdir(%q): %v", old, err)
		}
	}
}

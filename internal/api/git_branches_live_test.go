package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Uses a local go-git repo, not GitHub, so the test cannot flake on network availability.
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

	// Uses the unchecked lister: this test covers listing and filtering, not the scheme gate.
	branches, err := listRemoteBranchesUnchecked(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("listRemoteBranchesUnchecked() error = %v", err)
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

	// listRemoteBranchesUnchecked: see TestListRemoteBranches_Live's own
	// comment above for why this test uses the scheme-gate-free core
	// rather than listRemoteBranches itself.
	_, err := listRemoteBranchesUnchecked(context.Background(), nonexistent)
	if err == nil {
		t.Fatal("listRemoteBranchesUnchecked() error = nil, want an error for an unreachable repo")
	}
}

// go-git registers a file transport unconditionally, so without the scheme gate a file:// URL reads local repos.
func TestListRemoteBranches_SchemeRejected(t *testing.T) {
	srcDir := t.TempDir()
	if _, err := git.PlainInit(srcDir, false); err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	cases := []struct {
		name    string
		repoURL string
	}{
		{"file scheme", "file://" + srcDir},
		{"bare local path, no scheme", srcDir},
		{"ssh scheme", "ssh://git@internal.example.invalid/repo.git"},
		{"git scheme", "git://internal.example.invalid/repo.git"},
		{"malformed URL", "://not a url"},
		{"empty scheme with host-looking path", "internal.example.invalid/repo.git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			branches, err := listRemoteBranches(context.Background(), tc.repoURL)
			if err == nil {
				t.Fatalf("listRemoteBranches(%q) error = nil, want rejection; branches = %v", tc.repoURL, branches)
			}
			if branches != nil {
				t.Errorf("listRemoteBranches(%q) branches = %v, want nil on rejection", tc.repoURL, branches)
			}
		})
	}
}

// Proves the scheme gate lets http(s) URLs through; the fake server is not a real git backend, so the call still errors.
func TestListRemoteBranches_HTTPSchemeStillWorks(t *testing.T) {
	contacted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		contacted = true
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := listRemoteBranches(context.Background(), srv.URL+"/some/repo.git")
	if !contacted {
		t.Fatal("local http test server was never contacted: the scheme gate blocked a plain http:// URL")
	}
	if err == nil {
		t.Fatal("want an error: the fake server isn't a real git smart-HTTP backend")
	}
}

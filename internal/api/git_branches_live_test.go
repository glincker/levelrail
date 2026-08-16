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

	// listRemoteBranchesUnchecked, not listRemoteBranches: srcDir is a
	// bare filesystem path with no scheme, which listRemoteBranches
	// itself now rejects (see TestListRemoteBranches_SchemeRejected).
	// This test's own purpose is exercising the listing/filtering logic
	// network-free, not the scheme gate.
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

// TestListRemoteBranches_SchemeRejected is the live proof for this
// task's SSRF/local-file finding: go-git's transport client registry
// registers a "file" transport unconditionally (see
// errRepoURLSchemeNotAllowed's own doc comment in git_branches.go), so
// without the scheme gate a file:// repoURL really does read a local
// git repository on this host, and an http(s):// repoURL against an
// internal address really does fire a request. This proves the gate
// rejects every non-http(s) scheme before any transport is
// constructed: no local repo is ever actually reached (proven by
// srcDir being a real, listable repo in TestListRemoteBranches_Live
// above, contrasted with the same path via a file:// URL being
// rejected here with zero branches returned), and no network call is
// ever attempted for ssh/git schemes or a malformed URL.
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

// TestListRemoteBranches_HTTPSchemeStillWorks proves the scheme gate
// added for the SSRF/local-file finding doesn't collaterally break a
// real http(s) repo_url: a plain httptest.Server stands in for "some
// http(s) remote," and this asserts the request really reaches it (the
// gate lets it through) even though the fake server isn't a real git
// smart-HTTP backend and the call still ultimately errors.
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

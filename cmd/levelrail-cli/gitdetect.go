package main

import (
	"github.com/go-git/go-git/v5"
)

// detectedGit is what detectLocalGit found in the current directory's
// git checkout, if any. Either field may be empty independently: a repo
// with no "origin" remote still has a resolvable branch name, and vice
// versa.
type detectedGit struct {
	RepoURL string
	Ref     string
}

// detectLocalGit is the CLI's flyctl/railway-style "run this from
// inside your project and it figures out the repo" convenience: when
// --repo isn't given, apps create tries the current directory's own
// "origin" remote and current branch before giving up and asking the
// caller to supply --repo explicitly. Best-effort only: any failure
// (not a git repo, no "origin" remote, detached HEAD) returns a zero
// value and no error, since not being able to auto-detect is the normal
// case for a caller that isn't standing inside a checkout at all (an
// image registration, a CI runner with a fresh checkout under a
// different remote name), not a failure this CLI should ever surface as
// one.
func detectLocalGit() detectedGit {
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return detectedGit{}
	}

	var out detectedGit
	if remote, err := repo.Remote("origin"); err == nil {
		urls := remote.Config().URLs
		if len(urls) > 0 {
			out.RepoURL = urls[0]
		}
	}
	if head, err := repo.Head(); err == nil && head.Name().IsBranch() {
		out.Ref = head.Name().Short()
	}
	return out
}

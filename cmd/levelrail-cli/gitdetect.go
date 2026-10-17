package main

import (
	"github.com/go-git/go-git/v5"
)

// Either field may be empty on its own.
type detectedGit struct {
	RepoURL string
	Ref     string
}

// Best-effort: any failure returns zero values, since CI and image callers are often not in a checkout.
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

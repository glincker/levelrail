package main

import (
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

// Either field may be empty on its own.
type detectedGit struct {
	RepoURL string
	Ref     string
}

// gitSource adds where the inspected directory sits inside its repository.
type gitSource struct {
	detectedGit
	// RelDir is the directory's slash path under the repo root, empty at the root.
	RelDir string
}

// Best-effort: any failure returns zero values, since CI and image callers are often not in a checkout.
func detectLocalGit() detectedGit {
	return detectGitAt(".").detectedGit
}

func detectGitAt(dir string) gitSource {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return gitSource{}
	}

	var out gitSource
	if remote, err := repo.Remote("origin"); err == nil {
		urls := remote.Config().URLs
		if len(urls) > 0 {
			out.RepoURL = urls[0]
		}
	}
	if head, err := repo.Head(); err == nil && head.Name().IsBranch() {
		out.Ref = head.Name().Short()
	}
	if wt, err := repo.Worktree(); err == nil {
		out.RelDir = relUnderRoot(wt.Filesystem.Root(), dir)
	}
	return out
}

func relUnderRoot(root, dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if d, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = d
	}
	rel, err := filepath.Rel(root, absDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitFetcher reads pipeline files with a shallow, in-memory clone, so
// nothing is written to disk and no checkout is needed.
type GitFetcher struct{}

// FetchFiles clones branch (the remote default when empty) at depth 1 and
// returns the *.yaml and *.yml files of the first directory in dirs that
// exists at the tip commit.
func (GitFetcher) FetchFiles(ctx context.Context, url, token, branch string, dirs []string) (RepoFiles, error) {
	opts := &git.CloneOptions{URL: url, Depth: 1, SingleBranch: true, Tags: git.NoTags}
	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	if token != "" {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	repo, err := git.CloneContext(ctx, memory.NewStorage(), nil, opts)
	if err != nil {
		return RepoFiles{}, fmt.Errorf("clone %q: %w", url, err)
	}
	head, err := repo.Head()
	if err != nil {
		return RepoFiles{}, fmt.Errorf("resolve head: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return RepoFiles{}, fmt.Errorf("load commit %s: %w", head.Hash(), err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return RepoFiles{}, fmt.Errorf("load tree: %w", err)
	}
	out := RepoFiles{SHA: head.Hash().String(), Files: map[string][]byte{}}
	for _, d := range dirs {
		sub, err := tree.Tree(filepathToSlash(d))
		if errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
			continue
		}
		if err != nil {
			return RepoFiles{}, fmt.Errorf("read %s: %w", d, err)
		}
		out.Dir = d
		for _, e := range sub.Entries {
			ext := strings.ToLower(path.Ext(e.Name))
			if e.Mode == filemode.Dir || (ext != ".yaml" && ext != ".yml") {
				continue
			}
			f, err := sub.File(e.Name)
			if err != nil {
				return RepoFiles{}, fmt.Errorf("read %s/%s: %w", d, e.Name, err)
			}
			body, err := f.Contents()
			if err != nil {
				return RepoFiles{}, fmt.Errorf("read %s/%s: %w", d, e.Name, err)
			}
			out.Files[e.Name] = []byte(body)
		}
		return out, nil
	}
	return out, nil
}

func filepathToSlash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

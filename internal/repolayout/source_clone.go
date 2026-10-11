package repolayout

import (
	"context"
	"fmt"
	"io"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// cloneSource reads one commit's tree from an in-memory, depth-1 clone. It is
// the fallback for hosts with no tree-listing API support here.
type cloneSource struct {
	tree *object.Tree
}

// NewCloneSource fetches branch of repoURL at depth 1 into memory. Nothing is
// written to disk.
func NewCloneSource(ctx context.Context, repoURL, branch, token string) (Source, error) {
	opts := &git.CloneOptions{URL: repoURL, Depth: 1, SingleBranch: true, Tags: git.NoTags}
	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	if token != "" {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	repo, err := git.CloneContext(ctx, memory.NewStorage(), nil, opts)
	if err != nil {
		return nil, fmt.Errorf("repolayout: shallow clone: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("repolayout: resolve head: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("repolayout: load commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("repolayout: load tree: %w", err)
	}
	return &cloneSource{tree: tree}, nil
}

func (s *cloneSource) Paths(_ context.Context) ([]string, bool, error) {
	var out []string
	err := s.tree.Files().ForEach(func(f *object.File) error {
		out = append(out, f.Name)
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("walk tree: %w", err)
	}
	return out, false, nil
}

func (s *cloneSource) Read(_ context.Context, p string, limit int) ([]byte, error) {
	f, err := s.tree.File(p)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}
	r, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(io.LimitReader(r, int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	return b, nil
}

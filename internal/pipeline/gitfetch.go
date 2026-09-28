package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitFetcher reads pipeline files with a shallow, in-memory clone, so
// nothing is written to disk and no checkout is needed.
type GitFetcher struct{}

// shaFallbackDepth bounds the branch history fetched when a server refuses a
// fetch of one exact commit.
const shaFallbackDepth = 100

// FetchFiles returns the *.yaml and *.yml files of the first directory in dirs
// that exists. With sha empty it reads the tip of branch (the remote default
// when empty); otherwise it reads that exact commit and returns
// ErrSHAUnavailable when the commit cannot be fetched.
func (GitFetcher) FetchFiles(ctx context.Context, url, token, branch, sha string, dirs []string) (RepoFiles, error) {
	var auth transport.AuthMethod
	if token != "" {
		auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	var (
		repo *git.Repository
		hash plumbing.Hash
		err  error
	)
	if sha == "" {
		repo, hash, err = cloneTip(ctx, url, branch, auth)
	} else {
		repo, hash, err = fetchCommit(ctx, url, branch, sha, auth)
	}
	if err != nil {
		return RepoFiles{}, err
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return RepoFiles{}, fmt.Errorf("load commit %s: %w", hash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return RepoFiles{}, fmt.Errorf("load tree: %w", err)
	}
	return readPipelineDir(tree, hash.String(), dirs)
}

func cloneTip(ctx context.Context, url, branch string, auth transport.AuthMethod) (*git.Repository, plumbing.Hash, error) {
	opts := &git.CloneOptions{URL: url, Depth: 1, SingleBranch: true, Tags: git.NoTags, Auth: auth}
	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	repo, err := git.CloneContext(ctx, memory.NewStorage(), nil, opts)
	if err != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("clone %q: %w", url, err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("resolve head: %w", err)
	}
	return repo, head.Hash(), nil
}

// fetchCommit fetches exactly sha at depth 1, falling back to recent branch
// history for servers that only serve advertised refs.
func fetchCommit(ctx context.Context, url, branch, sha string, auth transport.AuthMethod) (*git.Repository, plumbing.Hash, error) {
	if !plumbing.IsHash(sha) {
		return nil, plumbing.ZeroHash, fmt.Errorf("%w: %q is not a commit id", ErrSHAUnavailable, sha)
	}
	hash := plumbing.NewHash(sha)
	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("init repo: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{url}}); err != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("add remote: %w", err)
	}
	exact := config.RefSpec(sha + ":refs/pipeline/sha")
	err = repo.FetchContext(ctx, &git.FetchOptions{RemoteName: "origin", RefSpecs: []config.RefSpec{exact}, Depth: 1, Tags: git.NoTags, Auth: auth})
	if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		if _, cerr := repo.CommitObject(hash); cerr == nil {
			return repo, hash, nil
		}
	}
	if ctx.Err() != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("fetch %s: %w", sha, ctx.Err())
	}
	opts := &git.CloneOptions{URL: url, Depth: shaFallbackDepth, SingleBranch: true, Tags: git.NoTags, Auth: auth}
	if branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	repo, cerr := git.CloneContext(ctx, memory.NewStorage(), nil, opts)
	if cerr != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("%w: %s: %v", ErrSHAUnavailable, sha, cerr)
	}
	if _, cerr := repo.CommitObject(hash); cerr != nil {
		return nil, plumbing.ZeroHash, fmt.Errorf("%w: %s", ErrSHAUnavailable, sha)
	}
	return repo, hash, nil
}

// readPipelineDir enforces maxSyncFiles and maxSyncFileSize while reading, so
// a huge directory is never held in memory. One file past the cap is kept so
// the syncer can report the overflow.
func readPipelineDir(tree *object.Tree, sha string, dirs []string) (RepoFiles, error) {
	out := RepoFiles{SHA: sha, Files: map[string][]byte{}, Oversize: map[string]int64{}}
	for _, d := range dirs {
		sub, err := tree.Tree(filepathToSlash(d))
		if errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
			continue
		}
		if err != nil {
			return RepoFiles{}, fmt.Errorf("read %s: %w", d, err)
		}
		out.Dir = d
		counted := 0
		for _, e := range sub.Entries {
			ext := strings.ToLower(path.Ext(e.Name))
			if e.Mode == filemode.Dir || (ext != ".yaml" && ext != ".yml") {
				continue
			}
			if counted > maxSyncFiles {
				break
			}
			counted++
			f, err := sub.File(e.Name)
			if err != nil {
				return RepoFiles{}, fmt.Errorf("read %s/%s: %w", d, e.Name, err)
			}
			if f.Size > maxSyncFileSize {
				out.Oversize[e.Name] = f.Size
				continue
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

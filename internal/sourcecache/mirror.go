package sourcecache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

const remoteName = "origin"

var fetchSpecs = []config.RefSpec{
	"+refs/heads/*:refs/remotes/origin/*",
	"+refs/tags/*:refs/tags/*",
}

func defaultFetch(ctx context.Context, repo *git.Repository, auth transport.AuthMethod) error {
	err := repo.FetchContext(ctx, &git.FetchOptions{
		RemoteName: remoteName, RefSpecs: fetchSpecs, Auth: auth, Tags: git.AllTags, Force: true,
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// refresh fetches repoURL into the mirror, retrying transient failures with
// exponential backoff. Fetches of one mirror are serialized.
func (c *Cache) refresh(ctx context.Context, repoURL, token, mirrorDir string, repo *git.Repository) error {
	if c.cfg.Dir != "" {
		st := c.stateFor(filepath.Base(mirrorDir))
		st.fetchMu.Lock()
		defer st.fetchMu.Unlock()
	}
	if _, err := repo.Remote(remoteName); err != nil {
		if _, cerr := repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{cleanURL(repoURL)}}); cerr != nil {
			return fmt.Errorf("sourcecache: configure mirror remote: %w", cerr)
		}
	}
	auth := authFor(repoURL, token)
	var err error
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, c.cfg.FetchTimeout)
		err = c.fetchFn(actx, repo, auth)
		cancel()
		if err == nil {
			c.health.markOK(repoURL)
			c.pruneQuietly()
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("sourcecache: fetch %s: %w", hostOf(repoURL), ctx.Err())
		}
		if !IsUnreachable(err) {
			return fmt.Errorf("sourcecache: fetch %s: %w", hostOf(repoURL), err)
		}
		if attempt >= c.cfg.Retries {
			break
		}
		c.log.Warn("sourcecache: git host fetch failed, retrying", "host", hostOf(repoURL), "attempt", attempt+1, "error", err.Error())
		if serr := c.sleep(ctx, c.cfg.Backoff<<attempt); serr != nil {
			return fmt.Errorf("sourcecache: fetch %s: %w", hostOf(repoURL), serr)
		}
	}
	c.health.markFailed(repoURL, c.now(), err.Error())
	return fmt.Errorf("%w (%s: %s)", ErrHostUnreachable, hostOf(repoURL), oneLine(err.Error()))
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// exportWorktree builds a self-contained repository in dest at commit: the
// mirror's objects are hard-linked (copied across filesystems), tags are
// carried over, and HEAD is detached at commit, so git commands in a build
// see the same history a full clone would.
func exportWorktree(mirrorDir string, commit plumbing.Hash, dest string) error {
	mirror, err := git.PlainOpen(mirrorDir)
	if err != nil {
		return err
	}
	if _, err := mirror.CommitObject(commit); err != nil {
		return err
	}
	repo, err := git.PlainInit(dest, false)
	if err != nil {
		return err
	}
	if err := linkTree(filepath.Join(mirrorDir, "objects"), filepath.Join(dest, ".git", "objects")); err != nil {
		return err
	}
	tags, err := mirror.Tags()
	if err != nil {
		return err
	}
	if err := tags.ForEach(func(r *plumbing.Reference) error {
		return repo.Storer.SetReference(plumbing.NewHashReference(r.Name(), r.Hash()))
	}); err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Checkout(&git.CheckoutOptions{Hash: commit, Force: true})
}

func linkTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if rel == filepath.Join("info", "alternates") || !d.Type().IsRegular() {
			return nil
		}
		if os.Link(path, target) == nil { //nolint:gosec // the mirror is written only by this process, not by untrusted users
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // both paths are inside the cache and checkout dirs
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o444) //nolint:gosec // see above
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

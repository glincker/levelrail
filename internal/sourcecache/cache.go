package sourcecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

const lastUsedFile = ".last_used"

// Checkout is a source tree materialized from the mirror. Cleanup removes
// the working tree and releases the mirror for pruning.
type Checkout struct {
	Dir    string
	Commit string
	// FromCache is true when no network call was made.
	FromCache bool
	cleanup   func()
}

// Cleanup is safe to call more than once.
func (c Checkout) Cleanup() {
	if c.cleanup != nil {
		c.cleanup()
	}
}

type offlineKey struct{}

// WithOffline makes Checkout resolve only from the mirror and never contact
// the git host.
func WithOffline(ctx context.Context) context.Context {
	return context.WithValue(ctx, offlineKey{}, true)
}

// IsOffline reports whether ctx came from WithOffline.
func IsOffline(ctx context.Context) bool {
	v, _ := ctx.Value(offlineKey{}).(bool)
	return v
}

type scopeKey struct{}

// WithScope names the credential identity a Checkout runs under (never the
// token value, which rotates). Mirrors are only shared within one scope.
func WithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func scopeOf(ctx context.Context) string {
	v, _ := ctx.Value(scopeKey{}).(string)
	return v
}

type mirrorState struct {
	fetchMu sync.Mutex
	users   int
}

// Cache is the bounded source mirror plus the git host health it observes.
type Cache struct {
	cfg    Config
	log    *slog.Logger
	health *health

	mu      sync.Mutex
	mirrors map[string]*mirrorState

	fetchFn func(ctx context.Context, repo *git.Repository, auth transport.AuthMethod) error
	sleep   func(ctx context.Context, d time.Duration) error
	probeFn func(ctx context.Context, repoURL string) error
	now     func() time.Time
	export  func(mirrorDir string, commit plumbing.Hash, dest string) error
}

// New builds a Cache; a nil logger discards logs.
func New(cfg Config, log *slog.Logger) *Cache {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	c := &Cache{cfg: cfg.withDefaults(), log: log, health: newHealth(), mirrors: map[string]*mirrorState{}, now: time.Now}
	c.fetchFn = defaultFetch
	c.sleep = sleepCtx
	c.probeFn = defaultProbe
	c.export = exportWorktree
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Checkout materializes ref of repoURL into a fresh temp directory. A full
// commit hash already in the mirror never touches the network. Any other
// ref is refreshed first, so a moved branch is never served stale; when the
// host is down that fails with ErrHostUnreachable. Under WithOffline only
// the mirror is read.
func (c *Cache) Checkout(ctx context.Context, repoURL, ref, token string) (Checkout, error) {
	mirrorDir, release, err := c.acquire(repoURL, scopeOf(ctx))
	if err != nil {
		return Checkout{}, err
	}
	fail := func(err error) (Checkout, error) {
		release()
		return Checkout{}, err
	}

	repo, err := c.openMirror(mirrorDir)
	if err != nil {
		return fail(err)
	}
	hash, ok := resolve(repo, ref)
	fromCache := ok && isFullHash(ref)
	if IsOffline(ctx) {
		if !ok {
			return fail(fmt.Errorf("%w: %s", ErrNotCached, shortRef(ref)))
		}
		fromCache = true
	} else if !fromCache {
		if err := c.refresh(ctx, repoURL, token, mirrorDir, repo); err != nil {
			return fail(err)
		}
		if repo, err = git.PlainOpen(mirrorDir); err != nil {
			return fail(fmt.Errorf("sourcecache: reopen mirror: %w", err))
		}
		if hash, ok = resolve(repo, ref); !ok {
			return fail(fmt.Errorf("%w: %s", ErrCommitNotFound, shortRef(ref)))
		}
	}

	dest, err := os.MkdirTemp("", "levelrail-src-*")
	if err != nil {
		return fail(fmt.Errorf("sourcecache: create checkout dir: %w", err))
	}
	if err := c.export(mirrorDir, hash, dest); err != nil {
		_ = os.RemoveAll(dest)
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return fail(fmt.Errorf("%w: %s", ErrCommitNotFound, shortRef(ref)))
		}
		return fail(fmt.Errorf("sourcecache: check out %s: %w", shortRef(ref), err))
	}
	c.touch(mirrorDir)
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if rmErr := os.RemoveAll(dest); rmErr != nil {
				c.log.Error("sourcecache: remove checkout dir failed", slog.String("dir", dest), slog.String("error", rmErr.Error()))
			}
			release()
		})
	}
	return Checkout{Dir: dest, Commit: hash.String(), FromCache: fromCache, cleanup: cleanup}, nil
}

// acquire returns the mirror directory for repoURL and marks it in use.
func (c *Cache) acquire(repoURL, scope string) (dir string, release func(), err error) {
	base := c.cfg.Dir
	if base == "" {
		tmp, err := os.MkdirTemp("", "levelrail-mirror-*")
		if err != nil {
			return "", nil, fmt.Errorf("sourcecache: create temp mirror: %w", err)
		}
		return filepath.Join(tmp, "m"), func() { _ = os.RemoveAll(tmp) }, nil
	}
	key := mirrorKey(repoURL, scope)
	st := c.stateFor(key)
	c.mu.Lock()
	st.users++
	c.mu.Unlock()
	var once sync.Once
	return filepath.Join(base, key), func() {
		once.Do(func() {
			c.mu.Lock()
			st.users--
			c.mu.Unlock()
		})
	}, nil
}

func (c *Cache) stateFor(key string) *mirrorState {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.mirrors[key]
	if st == nil {
		st = &mirrorState{}
		c.mirrors[key] = st
	}
	return st
}

func (c *Cache) openMirror(dir string) (*git.Repository, error) {
	if repo, err := git.PlainOpen(dir); err == nil {
		return repo, nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("sourcecache: create mirror dir: %w", err)
	}
	repo, err := git.PlainInit(dir, true)
	if err != nil {
		return nil, fmt.Errorf("sourcecache: init mirror: %w", err)
	}
	return repo, nil
}

func (c *Cache) touch(dir string) {
	p := filepath.Join(dir, lastUsedFile)
	now := c.now()
	if err := os.WriteFile(p, nil, 0o600); err == nil {
		_ = os.Chtimes(p, now, now)
	}
}

func isFullHash(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

func shortRef(ref string) string {
	if isFullHash(ref) {
		return ref[:12]
	}
	return ref
}

func resolve(repo *git.Repository, ref string) (plumbing.Hash, bool) {
	if isFullHash(ref) {
		h := plumbing.NewHash(ref)
		if _, err := repo.CommitObject(h); err != nil {
			return plumbing.ZeroHash, false
		}
		return h, true
	}
	for _, rev := range []string{ref, "refs/remotes/origin/" + ref} {
		if h, err := repo.ResolveRevision(plumbing.Revision(rev)); err == nil {
			if cm, cerr := repo.CommitObject(*h); cerr == nil {
				return cm.Hash, true
			}
		}
	}
	return plumbing.ZeroHash, false
}

// pushMirrorPrefix marks mirrors filled by git push, which hold local data
// and no remote credential, so they keep their original key.
const pushMirrorPrefix = "push://local/"

// mirrorKey separates mirrors by credential scope so a cached private commit
// is only served to callers using the credential that fetched it.
func mirrorKey(repoURL, scope string) string {
	if strings.HasPrefix(repoURL, pushMirrorPrefix) {
		sum := sha256.Sum256([]byte(normalizeURL(repoURL)))
		return hex.EncodeToString(sum[:16])
	}
	sum := sha256.Sum256([]byte(normalizeURL(repoURL) + "\x00" + scope))
	return hex.EncodeToString(sum[:16])
}

func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(raw, "/"), ".git"))
	}
	p := strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
	return strings.ToLower(u.Scheme + "://" + u.Host + p)
}

func authFor(repoURL, token string) transport.AuthMethod {
	if token != "" {
		return &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	if u, err := url.Parse(repoURL); err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok {
			return &githttp.BasicAuth{Username: u.User.Username(), Password: pw}
		}
	}
	return nil
}

// cleanURL drops any credentials so they are never written to the mirror.
func cleanURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

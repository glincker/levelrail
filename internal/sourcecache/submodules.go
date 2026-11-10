package sourcecache

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// Errors a submodule or LFS fetch reports; each message names the module or
// host, never a URL that could carry a token.
var (
	ErrSubmoduleURL      = errors.New("sourcecache: submodule url refused")
	ErrSubmodulePath     = errors.New("sourcecache: submodule path refused")
	ErrSubmoduleAuth     = errors.New("sourcecache: submodule needs credentials")
	ErrSubmoduleLimit    = errors.New("sourcecache: submodule limit reached")
	ErrSubmoduleNotFound = errors.New("sourcecache: submodule commit not found")
	ErrLFSRefused        = errors.New("sourcecache: lfs download refused")
	// ErrSubmoduleUnreachable hides why a host failed (refused, timeout or a
	// blocked internal address); the server log keeps the cause.
	ErrSubmoduleUnreachable = errors.New("sourcecache: submodule host unreachable")
	ErrLFSFetch             = errors.New("sourcecache: lfs fetch failed")
	errExtrasNeedsNetwork   = errors.New("submodules and lfs objects need the git host")
)

// Extras asks a Checkout to also fetch submodules and LFS objects. The caps
// come from runtime settings; a zero cap refuses any submodule or object.
type Extras struct {
	Submodules bool
	LFS        bool
	MaxDepth   int
	MaxCount   int
	MaxBytes   int64
	Timeout    time.Duration
	// CredentialHosts get the parent's credentials too; any other host
	// than the parent's is fetched anonymously.
	CredentialHosts []string
	// LFSHosts may serve LFS downloads besides the parent host.
	LFSHosts []string
	// AllowedCIDRs are internal networks submodule and LFS hosts may be on;
	// every other private, loopback or link-local address is refused.
	AllowedCIDRs []string
}

type extrasKey struct{}

// WithExtras attaches submodule and LFS options to a Checkout.
func WithExtras(ctx context.Context, ex Extras) context.Context {
	return context.WithValue(ctx, extrasKey{}, ex)
}

func extrasOf(ctx context.Context) (Extras, bool) {
	ex, ok := ctx.Value(extrasKey{}).(Extras)
	return ex, ok && (ex.Submodules || ex.LFS)
}

type subFetchFunc func(ctx context.Context, dir, repoURL string, auth transport.AuthMethod) error

// defaultSubmoduleFetch fetches every branch and tag of an https repoURL
// into a new repository at dir over the guarded client (policy from ctx).
func defaultSubmoduleFetch(ctx context.Context, dir, repoURL string, auth transport.AuthMethod) error {
	if !strings.HasPrefix(repoURL, "https://") {
		return errors.New("only https is allowed")
	}
	installSubProtocol()
	return fetchInto(ctx, dir, subScheme+strings.TrimPrefix(repoURL, "https"), repoURL, auth)
}

// fetchInto fetches from fetchURL, then records storedURL as the remote.
func fetchInto(ctx context.Context, dir, fetchURL, storedURL string, auth transport.AuthMethod) error {
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		return err
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{fetchURL}}); err != nil {
		return err
	}
	err = repo.FetchContext(ctx, &git.FetchOptions{RemoteName: remoteName, RefSpecs: fetchSpecs, Auth: auth, Tags: git.AllTags, Force: true})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		err = nil
	}
	if err != nil {
		return err
	}
	if derr := repo.DeleteRemote(remoteName); derr != nil {
		return derr
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{storedURL}})
	return err
}

type extrasRun struct {
	c          *Cache
	ex         Extras
	root       string
	parentHost string
	auth       transport.AuthMethod
	allowed    []netip.Prefix
	count      int
	repos      []repoDir
}

type repoDir struct{ dir, url string }

// fetchExtras fills dest's submodules (recursively, within the caps) and
// then replaces LFS pointer files with their objects.
func (c *Cache) fetchExtras(ctx context.Context, dest, parentURL string, auth transport.AuthMethod, ex Extras) error {
	if ex.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ex.Timeout)
		defer cancel()
	}
	clean := cleanURL(parentURL)
	run := &extrasRun{c: c, ex: ex, root: dest, parentHost: urlHost(clean), auth: auth, allowed: parseCIDRs(ex.AllowedCIDRs), repos: []repoDir{{dest, clean}}}
	if ex.Submodules {
		if err := run.submodules(ctx, dest, clean, 1); err != nil {
			return err
		}
	}
	if ex.LFS {
		for _, r := range run.repos {
			if err := run.lfs(ctx, r.dir, r.url); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *extrasRun) submodules(ctx context.Context, dir, repoURL string, depth int) error {
	data, err := os.ReadFile(filepath.Join(dir, ".gitmodules")) //nolint:gosec // dir is inside the checkout this run created
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sourcecache: read .gitmodules: %w", err)
	}
	mods := config.NewModules()
	if err := mods.Unmarshal(data); err != nil {
		return fmt.Errorf("sourcecache: parse .gitmodules: %w", err)
	}
	if len(mods.Submodules) == 0 {
		return nil
	}
	if depth > r.ex.MaxDepth {
		return fmt.Errorf("%w: submodules nest deeper than %d", ErrSubmoduleLimit, r.ex.MaxDepth)
	}
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return fmt.Errorf("sourcecache: open checkout: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("sourcecache: read checkout head: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("sourcecache: read checkout commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("sourcecache: read checkout tree: %w", err)
	}
	for _, m := range mods.Submodules {
		rel, err := safeSubmodulePath(m.Path)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", ErrSubmodulePath, m.Name, err)
		}
		entry, err := tree.FindEntry(filepath.ToSlash(rel))
		if err != nil || entry.Mode != filemode.Submodule {
			continue // listed in .gitmodules but not pinned in the tree, git skips it too
		}
		subURL, err := resolveSubmoduleURL(repoURL, m.URL)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", ErrSubmoduleURL, m.Name, err)
		}
		r.count++
		if r.count > r.ex.MaxCount {
			return fmt.Errorf("%w: more than %d submodules", ErrSubmoduleLimit, r.ex.MaxCount)
		}
		target, err := r.prepareDir(dir, rel)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", ErrSubmodulePath, m.Name, err)
		}
		if err := r.fetchOne(ctx, m.Name, target, subURL, entry.Hash); err != nil {
			return err
		}
		if size := dirSize(r.root); size > r.ex.MaxBytes {
			return fmt.Errorf("%w: checkout with submodules is over %d bytes", ErrSubmoduleLimit, r.ex.MaxBytes)
		}
		r.repos = append(r.repos, repoDir{target, subURL.String()})
		if err := r.submodules(ctx, target, subURL.String(), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (r *extrasRun) fetchOne(ctx context.Context, name, target string, u *url.URL, commit plumbing.Hash) error {
	host := strings.ToLower(u.Host)
	var auth transport.AuthMethod
	if r.credentialsFor(host) {
		auth = r.auth
	}
	fetch := r.c.subFetch
	if fetch == nil {
		fetch = defaultSubmoduleFetch
	}
	budget := r.budget()
	if err := fetch(withPolicy(ctx, &netPolicy{allowed: r.allowed, budget: budget}), target, u.String(), auth); err != nil {
		if errors.Is(err, errBudgetExceeded) || budget.Load() < 0 {
			return fmt.Errorf("%w: checkout with submodules is over %d bytes", ErrSubmoduleLimit, r.ex.MaxBytes)
		}
		if errors.Is(err, transport.ErrAuthenticationRequired) || errors.Is(err, transport.ErrAuthorizationFailed) || errors.Is(err, transport.ErrRepositoryNotFound) {
			hint := "the app's git credentials cannot read it"
			if auth == nil {
				hint = "credentials are only sent to the parent repository host; add this host to git.submodules_credential_hosts to send them"
			}
			return fmt.Errorf("%w: %q on %s: %s", ErrSubmoduleAuth, name, host, hint)
		}
		return r.c.unreachable("submodule", host, err)
	}
	repo, err := git.PlainOpen(target)
	if err != nil {
		return fmt.Errorf("sourcecache: open submodule %q: %w", name, err)
	}
	if _, err := repo.CommitObject(commit); err != nil {
		return fmt.Errorf("%w: %q at %s on %s", ErrSubmoduleNotFound, name, shortRef(commit.String()), host)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("sourcecache: submodule %q worktree: %w", name, err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Hash: commit, Force: true}); err != nil {
		return fmt.Errorf("sourcecache: check out submodule %q: %w", name, err)
	}
	return nil
}

// budget is what the size cap leaves for the next download.
func (r *extrasRun) budget() *atomic.Int64 {
	b := &atomic.Int64{}
	b.Store(r.ex.MaxBytes - dirSize(r.root))
	return b
}

func (r *extrasRun) credentialsFor(host string) bool {
	if host == r.parentHost {
		return true
	}
	return hostListed(r.ex.CredentialHosts, host)
}

func hostListed(list []string, host string) bool {
	for _, h := range list {
		if strings.EqualFold(strings.TrimSpace(h), host) {
			return true
		}
	}
	return false
}

// prepareDir returns the submodule directory under base, refusing any path
// whose components are symlinks so a fetch can never land outside the checkout.
func (r *extrasRun) prepareDir(base, rel string) (string, error) {
	cur := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(cur, 0o750); err != nil {
				return "", err
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return "", errors.New("path is not a plain directory")
		}
	}
	entries, err := os.ReadDir(cur)
	if err != nil {
		return "", err
	}
	if len(entries) > 0 {
		return "", errors.New("directory is not empty")
	}
	return cur, nil
}

// safeSubmodulePath accepts only a relative path that stays inside the
// checkout and never touches a .git directory.
func safeSubmodulePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsRune(p, 0) || strings.Contains(p, `\`) {
		return "", errors.New("empty or malformed path")
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(p) {
		return "", errors.New("absolute path")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", errors.New("path leaves the checkout")
		}
		if strings.EqualFold(seg, ".git") {
			return "", errors.New("path enters a .git directory")
		}
	}
	clean := path.Clean(p)
	if clean == "." {
		return "", errors.New("path is the checkout root")
	}
	return filepath.FromSlash(clean), nil
}

// resolveSubmoduleURL accepts https URLs without embedded credentials and
// ./ or ../ paths relative to the parent that stay on the parent host.
func resolveSubmoduleURL(parentURL, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, "\x00\n\r") {
		return nil, errors.New("empty or malformed url")
	}
	if strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		parent, err := url.Parse(parentURL)
		if err != nil || parent.Scheme != "https" || parent.Host == "" {
			return nil, errors.New("a relative url needs an https parent repository")
		}
		joined, ok := joinWithin(parent.Path, raw)
		if !ok {
			return nil, errors.New("relative url leaves the parent host")
		}
		return &url.URL{Scheme: "https", Host: parent.Host, Path: joined}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("not a url (only https is allowed)")
	}
	if u.Scheme != "https" {
		return nil, errors.New("only https urls are allowed")
	}
	if u.User != nil {
		return nil, errors.New("credentials in the url are not allowed")
	}
	if u.Host == "" || u.Opaque != "" {
		return nil, errors.New("url has no host")
	}
	if numericHost(u.Hostname()) {
		return nil, errors.New("numeric host forms are not allowed")
	}
	u.Fragment = ""
	return u, nil
}

// joinWithin resolves rel against the parent repository path the way git
// does (relative to the repository, not its directory) and reports an escape.
func joinWithin(parentPath, rel string) (string, bool) {
	segs := []string{}
	for _, s := range strings.Split(strings.Trim(parentPath, "/"), "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	for _, s := range strings.Split(rel, "/") {
		switch s {
		case "", ".":
		case "..":
			if len(segs) == 0 {
				return "", false
			}
			segs = segs[:len(segs)-1]
		default:
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return "", false
	}
	return "/" + strings.Join(segs, "/"), true
}

// numericHost catches decimal, octal and hex IPv4 spellings that would
// slip past a hostname review; real dotted or bracketed IPs are left to the dialer.
func numericHost(h string) bool {
	if _, err := netip.ParseAddr(h); err == nil {
		return false
	}
	h = strings.ToLower(h)
	if strings.HasPrefix(h, "0x") || strings.Contains(h, ".0x") {
		return true
	}
	return strings.Trim(h, "0123456789.") == ""
}

// httpClient refuses redirects to hosts the download itself would be refused on.
func (r *extrasRun) httpClient(repoHost string) *http.Client {
	base := r.c.lfsClient
	if base == nil {
		base = defaultLFSClient()
	}
	cl := *base
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" || !r.lfsHostAllowed(repoHost, strings.ToLower(req.URL.Host)) {
			return fmt.Errorf("%w: redirect to %s", ErrLFSRefused, req.URL.Host)
		}
		if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			req.Header.Del("Authorization")
		}
		return nil
	}
	return &cl
}

// lfsHostAllowed compares with the repository the pointer came from, so a
// submodule's objects are judged against its own host, not the parent's.
func (r *extrasRun) lfsHostAllowed(repoHost, host string) bool {
	return host == repoHost || hostListed(r.ex.LFSHosts, host)
}

// urlHost is the lowercased host:port; a port is part of the host identity.
func urlHost(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.ToLower(u.Host)
	}
	return ""
}

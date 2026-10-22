// Package repopreview builds the read-only repository card shown before an
// app is created from a git URL. It only talks to known git hosts.
package repopreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/appwizard"
)

// Env vars bounding one preview.
const (
	EnvTimeout     = "APP_REPO_PREVIEW_TIMEOUT"
	EnvCacheTTL    = "APP_REPO_PREVIEW_CACHE_TTL"
	EnvMaxBranches = "APP_REPO_PREVIEW_MAX_BRANCHES"
	EnvReadmeChars = "APP_REPO_PREVIEW_README_CHARS"
	EnvCacheMax    = "APP_REPO_PREVIEW_CACHE_MAX"
)

// Kind is the outcome class the UI switches on.
type Kind string

// Outcome classes. KindPublic means a preview was produced; Visibility says
// whether the repository is private.
const (
	KindPublic          Kind = "public"
	KindPrivate         Kind = "private_needs_account"
	KindNotFound        Kind = "not_found"
	KindRateLimited     Kind = "rate_limited"
	KindUnsupportedHost Kind = "unsupported_host"
	KindInvalidURL      Kind = "invalid_url"
)

var (
	// ErrNotFound means the host has no such repository, or hides it from this caller.
	ErrNotFound = errors.New("repopreview: repository not found")
	// ErrRateLimited means the host throttled the request.
	ErrRateLimited = errors.New("repopreview: host rate limited")
	// ErrNoMetadata means the host is known but exposes no metadata API here.
	ErrNoMetadata = errors.New("repopreview: no metadata api for host")
)

// Language is one entry of the languages bar.
type Language struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

// Commit is the latest commit on the previewed branch.
type Commit struct {
	SHA     string    `json:"sha"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	At      time.Time `json:"at"`
}

// Meta is what a host client returns, before capping and sanitising.
type Meta struct {
	Owner         string
	Name          string
	Description   string
	DefaultBranch string
	Branches      []string
	Private       bool
	Stars         *int
	Forks         *int
	Languages     map[string]int64
	LastCommit    *Commit
	License       string
	Topics        []string
	Readme        string
	SizeKB        *int
}

// Target identifies one repository on one host.
type Target struct {
	URL   string
	Host  string
	Owner string
	Repo  string
	Ref   string
}

// HostClient fetches metadata for a target. Implementations return
// ErrNotFound, ErrRateLimited or ErrNoMetadata for the matching outcomes.
type HostClient interface {
	Fetch(ctx context.Context, t Target, token string) (Meta, error)
}

// AnalyzeFunc runs the stack analysis for a repository and ref.
type AnalyzeFunc func(ctx context.Context, repoURL, ref, token string) (*appwizard.Analysis, error)

// Result is the preview payload.
type Result struct {
	Kind          Kind                `json:"kind"`
	Host          string              `json:"host,omitempty"`
	RepoURL       string              `json:"repo_url,omitempty"`
	Owner         string              `json:"owner,omitempty"`
	Name          string              `json:"name,omitempty"`
	Description   string              `json:"description,omitempty"`
	DefaultBranch string              `json:"default_branch,omitempty"`
	Branches      []string            `json:"branches,omitempty"`
	Visibility    string              `json:"visibility,omitempty"`
	Stars         *int                `json:"stars,omitempty"`
	Forks         *int                `json:"forks,omitempty"`
	Languages     []Language          `json:"languages,omitempty"`
	LastCommit    *Commit             `json:"last_commit,omitempty"`
	License       string              `json:"license,omitempty"`
	Topics        []string            `json:"topics,omitempty"`
	ReadmeExcerpt string              `json:"readme_excerpt,omitempty"`
	SizeKB        *int                `json:"size_kb,omitempty"`
	Analysis      *appwizard.Analysis `json:"analysis,omitempty"`
}

// Builder assembles previews. Hosts are checked before any request is made.
type Builder struct {
	Client  HostClient
	Analyze AnalyzeFunc
	// Extra lists connected self-hosted hosts (host[:port]).
	Extra func(ctx context.Context) []string
	// Now is swappable for tests.
	Now func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	res Result
	at  time.Time
}

var publicHosts = map[string]bool{"github.com": true, "gitlab.com": true, "bitbucket.org": true, "codeberg.org": true}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return def
}

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(name)); err == nil && d >= 0 {
		return d
	}
	return def
}

// ParseTarget splits a repository URL into host, owner and repo. Credentials,
// queries and fragments are dropped so they never reach a cache key or log.
func ParseTarget(raw, ref string) (Target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return Target{}, errors.New("repopreview: unusable repository url")
	}
	p := strings.Trim(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "/")
	i := strings.LastIndex(p, "/")
	if i <= 0 || i == len(p)-1 {
		return Target{}, errors.New("repopreview: url has no owner and repo")
	}
	clean := url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/" + p}
	return Target{URL: clean.String(), Host: strings.ToLower(u.Host), Owner: p[:i], Repo: p[i+1:], Ref: ref}, nil
}

func (b *Builder) hostKnown(ctx context.Context, host string) bool {
	if publicHosts[strings.TrimPrefix(host, "www.")] {
		return true
	}
	if b.Extra == nil {
		return false
	}
	for _, h := range b.Extra(ctx) {
		if h != "" && strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}

// Build returns the preview for rawURL. token may be empty. A host failure
// becomes the matching Kind so the UI can degrade; only unexpected errors return err.
func (b *Builder) Build(ctx context.Context, rawURL, ref, token string) (Result, error) {
	t, err := ParseTarget(rawURL, ref)
	if err != nil {
		return Result{Kind: KindInvalidURL}, nil
	}
	if !b.hostKnown(ctx, t.Host) {
		return Result{Kind: KindUnsupportedHost, Host: t.Host, RepoURL: t.URL}, nil
	}
	sum := sha256.Sum256([]byte(token))
	key := t.URL + "#" + t.Ref + "#" + hex.EncodeToString(sum[:8])
	if res, ok := b.cached(key); ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, envDuration(EnvTimeout, 15*time.Second))
	defer cancel()
	meta, err := b.Client.Fetch(ctx, t, token)
	res := Result{Host: t.Host, RepoURL: t.URL, Owner: t.Owner, Name: t.Repo}
	switch {
	case errors.Is(err, ErrNotFound):
		res.Kind = KindNotFound
		if token == "" {
			res.Kind = KindPrivate
		}
		return res, nil
	case errors.Is(err, ErrRateLimited):
		res.Kind = KindRateLimited
		return res, nil
	case errors.Is(err, ErrNoMetadata):
		res.Kind = KindUnsupportedHost
		return res, nil
	case err != nil:
		return Result{}, fmt.Errorf("repopreview: fetch %s: %w", t.URL, err)
	}
	b.fill(&res, meta)
	branch := t.Ref
	if branch == "" {
		branch = meta.DefaultBranch
	}
	if b.Analyze != nil {
		if a, aerr := b.Analyze(ctx, t.URL, branch, token); aerr == nil {
			res.Analysis = a
		}
	}
	b.store(key, res)
	return res, nil
}

func (b *Builder) fill(res *Result, m Meta) {
	res.Kind = KindPublic
	res.Visibility = "public"
	if m.Private {
		res.Visibility = "private"
	}
	if m.Owner != "" {
		res.Owner = m.Owner
	}
	if m.Name != "" {
		res.Name = m.Name
	}
	res.Description = clip(m.Description, 300)
	res.DefaultBranch = m.DefaultBranch
	res.Branches = m.Branches
	if limit := envInt(EnvMaxBranches, 50); len(res.Branches) > limit {
		res.Branches = res.Branches[:limit]
	}
	res.Stars, res.Forks, res.SizeKB = m.Stars, m.Forks, m.SizeKB
	res.Languages = percentages(m.Languages)
	if m.LastCommit != nil {
		c := *m.LastCommit
		c.Message = clip(firstLine(c.Message), 140)
		if len(c.SHA) > 7 {
			c.SHA = c.SHA[:7]
		}
		res.LastCommit = &c
	}
	res.License = clip(m.License, 60)
	res.Topics = m.Topics
	if len(res.Topics) > 12 {
		res.Topics = res.Topics[:12]
	}
	res.ReadmeExcerpt = Excerpt(m.Readme, envInt(EnvReadmeChars, 400))
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b *Builder) cached(key string) (Result, bool) {
	ttl := envDuration(EnvCacheTTL, 2*time.Minute)
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.cache[key]
	if !ok || b.now().Sub(e.at) > ttl {
		return Result{}, false
	}
	return e.res, true
}

func (b *Builder) store(key string, res Result) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cache == nil {
		b.cache = map[string]cacheEntry{}
	}
	if len(b.cache) >= envInt(EnvCacheMax, 256) {
		clear(b.cache)
	}
	b.cache[key] = cacheEntry{res: res, at: b.now()}
}

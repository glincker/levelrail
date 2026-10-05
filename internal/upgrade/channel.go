package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
)

// Update channels a running build can compare itself against.
// ChannelStable is GitHub's own "latest release" concept (unchanged
// default behavior); ChannelBeta and ChannelEdge follow release.yml's
// own image-channel job scheme: a hyphenated SemVer tag (e.g.
// v1.2.3-beta.1) gets :beta, a main push gets :edge.
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
	ChannelEdge   = "edge"
)

// ValidChannel reports whether c is one of the three recognized channels.
func ValidChannel(c string) bool {
	switch c {
	case ChannelStable, ChannelBeta, ChannelEdge:
		return true
	default:
		return false
	}
}

const githubRepo = "glincker/levelrail"

// DefaultCacheTTL is how long a Cache entry stays fresh before a caller
// should fetch again: shared by GET /api/v1/updates, updatecheck.Scheduler,
// and alerting's kind=version_skew evaluator, so none of them hits
// GitHub more often than this regardless of its own call cadence.
const DefaultCacheTTL = time.Hour

var channelHTTPClient = &http.Client{Timeout: 8 * time.Second}

// Release is the subset of a GitHub release, or (for the edge channel) a
// main-branch commit treated as a pseudo-release, that a channel-aware
// update check needs.
type Release struct {
	Tag         string
	URL         string
	PublishedAt string
	Body        string
	AssetNames  []string
}

type rawRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
	Assets      []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func (r rawRelease) toRelease() *Release {
	names := make([]string, 0, len(r.Assets))
	for _, a := range r.Assets {
		names = append(names, a.Name)
	}
	return &Release{Tag: r.TagName, URL: r.HTMLURL, PublishedAt: r.PublishedAt, Body: r.Body, AssetNames: names}
}

// githubGet decodes url's JSON body into out. A 404 reports its status
// with a nil error, the same "not found is a normal state, not a
// failure" convention internal/api/updates.go's defaultFetchLatestRelease
// already establishes.
func githubGet(ctx context.Context, url string, out any) (status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("upgrade: build github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := channelHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("upgrade: fetch github: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, fmt.Errorf("upgrade: github %s: unexpected status %d: %s", url, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("upgrade: decode github response: %w", err)
	}
	return resp.StatusCode, nil
}

// FetchLatestStable fetches GitHub's /releases/latest: the newest
// non-prerelease, non-draft release. With no stable release it falls back
// to the newest pre-release (install.sh's rule). nil, nil means nothing
// has been published at all.
func FetchLatestStable(ctx context.Context) (*Release, error) {
	var rr rawRelease
	status, err := githubGet(ctx, "https://api.github.com/repos/"+githubRepo+"/releases/latest", &rr)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return FetchLatestBeta(ctx)
	}
	return rr.toRelease(), nil
}

// FetchLatestBeta fetches the newest prerelease from GitHub's releases
// list, skipping drafts. nil, nil means no prerelease has ever been
// published.
func FetchLatestBeta(ctx context.Context) (*Release, error) {
	var rrs []rawRelease
	status, err := githubGet(ctx, "https://api.github.com/repos/"+githubRepo+"/releases?per_page=20", &rrs)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	return pickLatestPrerelease(rrs), nil
}

// pickLatestPrerelease returns the prerelease, non-draft entry in rrs
// with the newest published_at. GitHub's /releases list is ordered by
// internal release id, not publish time: observed live, v0.2.0-beta.9
// listed ahead of the actually-newer v0.2.0-beta.14, so taking the first
// match silently reported a nine-release-old "latest".
func pickLatestPrerelease(rrs []rawRelease) *Release {
	var latest *rawRelease
	var latestAt time.Time
	for i := range rrs {
		rr := rrs[i]
		if !rr.Prerelease || rr.Draft {
			continue
		}
		at, err := time.Parse(time.RFC3339, rr.PublishedAt)
		if err != nil {
			continue
		}
		if latest == nil || at.After(latestAt) {
			latest, latestAt = &rr, at
		}
	}
	if latest == nil {
		return nil
	}
	return latest.toRelease()
}

type commitResource struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Committer struct {
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

// mainCommitShortSHALen matches release.yml's own main-<sha> Docker
// image tag scheme closely enough to be recognizable, without needing
// the full 40-character SHA in a version string an operator reads.
const mainCommitShortSHALen = 7

// FetchLatestEdge fetches the latest commit on main and reports it as a
// pseudo-release tagged "main-<short sha>", matching release.yml's own
// image-channel job scheme for what a main push's Docker image tag is
// (IMAGE_VERSION: main-<sha>). nil, nil means the lookup found nothing
// (an empty or inaccessible repo).
func FetchLatestEdge(ctx context.Context) (*Release, error) {
	var c commitResource
	status, err := githubGet(ctx, "https://api.github.com/repos/"+githubRepo+"/commits/main", &c)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	sha := c.SHA
	if len(sha) > mainCommitShortSHALen {
		sha = sha[:mainCommitShortSHALen]
	}
	return &Release{Tag: "main-" + sha, URL: c.HTMLURL, PublishedAt: c.Commit.Committer.Date}, nil
}

// Fetchers groups one fetch function per channel, the same overridable
// seam Router.fetchLatestRelease (internal/api/updates.go) already
// establishes for testing without a real outbound call.
type Fetchers struct {
	Stable func(context.Context) (*Release, error)
	Beta   func(context.Context) (*Release, error)
	Edge   func(context.Context) (*Release, error)
}

// DefaultFetchers wires the three real GitHub lookups above.
func DefaultFetchers() Fetchers {
	return Fetchers{Stable: FetchLatestStable, Beta: FetchLatestBeta, Edge: FetchLatestEdge}
}

// LatestForChannel dispatches to the fetcher for channel, defaulting to
// Stable for an unrecognized or empty channel.
func (f Fetchers) LatestForChannel(ctx context.Context, channel string) (*Release, error) {
	switch channel {
	case ChannelBeta:
		return f.Beta(ctx)
	case ChannelEdge:
		return f.Edge(ctx)
	default:
		return f.Stable(ctx)
	}
}

// UpdateAvailable reports whether latest represents a newer build than
// currentVersion. A "dev" build never reports an update available:
// there is no meaningful newer build to compare an unreleased build
// against. Two SemVer tags compare by precedence (beta.9 < beta.15);
// anything else (edge's main-<sha>) falls back to inequality.
func UpdateAvailable(currentVersion string, latest *Release) bool {
	if latest == nil || currentVersion == "dev" {
		return false
	}
	cur, curErr := semver.NewVersion(currentVersion)
	lat, latErr := semver.NewVersion(latest.Tag)
	if curErr != nil || latErr != nil {
		return latest.Tag != currentVersion
	}
	return lat.GreaterThan(cur)
}

// Cache holds the last successful channel lookup, so GET /api/v1/updates
// and internal/updatecheck.Scheduler don't hit GitHub on every call.
// Keyed by channel (not just a bare TTL) so switching channels never
// serves a stale result cached for a different one.
type Cache struct {
	mu        sync.Mutex
	channel   string
	release   *Release
	fetched   bool
	fetchedAt time.Time
}

// NewCache returns an empty Cache, ready to use.
func NewCache() *Cache { return &Cache{} }

// Fresh returns the cached release for channel if one exists and is
// within ttl.
func (c *Cache) Fresh(ttl time.Duration, channel string) (*Release, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetched || c.channel != channel || time.Since(c.fetchedAt) > ttl {
		return nil, false
	}
	return c.release, true
}

// Stale returns whatever was last cached for channel, regardless of
// age, for use when a fresh fetch just failed.
func (c *Cache) Stale(channel string) (*Release, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetched || c.channel != channel {
		return nil, false
	}
	return c.release, true
}

// Set records release as the latest lookup result for channel.
func (c *Cache) Set(channel string, release *Release) {
	c.mu.Lock()
	c.channel = channel
	c.release = release
	c.fetched = true
	c.fetchedAt = time.Now()
	c.mu.Unlock()
}

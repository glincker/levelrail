package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/version"
)

// githubLatestReleaseURL is the public, unauthenticated GitHub Releases
// endpoint for this project's own repo.
const githubLatestReleaseURL = "https://api.github.com/repos/glincker/levelrail/releases/latest"

// updatesCacheTTL bounds how often handleGetUpdates hits GitHub's API,
// so a busy Settings > Updates page can't risk rate limiting.
const updatesCacheTTL = time.Hour

var updatesHTTPClient = &http.Client{Timeout: 8 * time.Second}

// githubRelease is the subset of GitHub's release object this handler
// needs.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// fetchLatestReleaseFunc matches defaultFetchLatestRelease's signature;
// overridable per-Router the same way lookupHost/fetch already are, so
// tests never perform a real outbound call. A nil *githubRelease with a
// nil error means "checked, no release published yet" (GitHub 404).
type fetchLatestReleaseFunc func(ctx context.Context) (*githubRelease, error)

func defaultFetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("api: build github releases request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := updatesHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api: fetch github releases: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("api: github releases: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("api: decode github release: %w", err)
	}
	return &release, nil
}

// updatesCache holds the last successful GitHub Releases lookup, so a
// failed outbound call can still serve a previous result instead of
// degrading to "unknown" every time GitHub is briefly unreachable.
type updatesCache struct {
	mu        sync.Mutex
	release   *githubRelease
	fetched   bool
	fetchedAt time.Time
}

func newUpdatesCache() *updatesCache {
	return &updatesCache{}
}

// fresh returns the cached release if one exists and is within ttl.
func (c *updatesCache) fresh(ttl time.Duration) (*githubRelease, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetched || time.Since(c.fetchedAt) > ttl {
		return nil, false
	}
	return c.release, true
}

// stale returns whatever was last cached, regardless of age, for use
// when a fresh fetch just failed.
func (c *updatesCache) stale() (*githubRelease, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.release, c.fetched
}

func (c *updatesCache) set(release *githubRelease) {
	c.mu.Lock()
	c.release = release
	c.fetched = true
	c.fetchedAt = time.Now()
	c.mu.Unlock()
}

// updateStatusResource is GET /api/v1/updates' wire shape. LatestVersion,
// ReleaseURL, and PublishedAt are all nil when no release has ever been
// published, or when GitHub couldn't be reached and nothing is cached
// yet: both are normal, non-error states, not distinguished in the
// response.
type updateStatusResource struct {
	CurrentVersion  string  `json:"current_version"`
	LatestVersion   *string `json:"latest_version"`
	UpdateAvailable bool    `json:"update_available"`
	ReleaseURL      *string `json:"release_url"`
	PublishedAt     *string `json:"published_at"`
}

// handleGetUpdates handles GET /api/v1/updates: the running version
// against GitHub's latest published release. AbilityRead, the same
// passive-visibility tier GET /api/v1/system/status uses. A dev build
// (version.Version == "dev") never reports update_available: there is no
// meaningful "newer" to compare against an unreleased build.
func (rt *Router) handleGetUpdates(w http.ResponseWriter, r *http.Request) {
	resp := updateStatusResource{CurrentVersion: version.Version}

	release, ok := rt.updatesCache.fresh(updatesCacheTTL)
	if !ok {
		fetched, err := rt.fetchLatestRelease(r.Context())
		if err != nil {
			rt.logger.Warn("api: fetch latest github release failed", slog.String("error", err.Error()))
			release, ok = rt.updatesCache.stale()
		} else {
			rt.updatesCache.set(fetched)
			release, ok = fetched, true
		}
	}

	if ok && release != nil {
		tag, url, published := release.TagName, release.HTMLURL, release.PublishedAt
		resp.LatestVersion = &tag
		resp.ReleaseURL = &url
		resp.PublishedAt = &published
		resp.UpdateAvailable = version.Version != "dev" && tag != version.Version
	}
	writeJSON(w, http.StatusOK, resp)
}

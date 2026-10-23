package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const githubRepo = "glincker/levelrail"

// updatesCacheTTL bounds how often handleGetUpdates hits GitHub's API,
// so a busy Settings > Updates page can't risk rate limiting.
const updatesCacheTTL = time.Hour

// githubRelease is the subset of GitHub's release object this handler
// needs.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Assets      []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// fetchLatestReleaseFunc matches defaultFetchLatestRelease's signature;
// overridable per-Router the same way lookupHost/fetch already are, so
// tests never perform a real outbound call. A nil *githubRelease with a
// nil error means "checked, no release published yet" (GitHub 404).
type fetchLatestReleaseFunc func(ctx context.Context) (*githubRelease, error)

// fetchStableRelease is swapped in tests; production resolves through kit/upgrade.
var fetchStableRelease = upgrade.FetchLatestStable

func defaultFetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	rel, err := fetchStableRelease(ctx, githubRepo)
	if err != nil {
		return nil, fmt.Errorf("api: fetch github releases: %w", err)
	}
	if rel == nil {
		return nil, nil
	}
	out := &githubRelease{TagName: rel.Tag, HTMLURL: rel.URL, PublishedAt: rel.PublishedAt, Body: rel.Body}
	for _, n := range rel.AssetNames {
		out.Assets = append(out.Assets, struct {
			Name string `json:"name"`
		}{Name: n})
	}
	return out, nil
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

// expire marks the cached release stale so the next lookup refetches, while
// stale() still serves it if that fetch fails.
func (c *updatesCache) expire() {
	c.mu.Lock()
	c.fetchedAt = time.Time{}
	c.mu.Unlock()
}

func (c *updatesCache) checkedAt() *string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetched {
		return nil
	}
	at := c.fetchedAt.UTC().Format(time.RFC3339)
	return &at
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
	// ReleaseKind is "release" or "edge_image"; Channel is the active one.
	ReleaseKind string `json:"release_kind"`
	Channel     string `json:"channel"`
	// Image is the container image reference an edge build is pulled from.
	Image *string `json:"image,omitempty"`
	// CheckedAt is when the release lookup last succeeded (stable channel).
	CheckedAt *string `json:"checked_at,omitempty"`
}

// handleGetUpdates handles GET /api/v1/updates: the running version
// against the configured channel's latest published release
// (update_settings.channel, migrations/0258, defaults to "stable").
// AbilityRead, the same passive-visibility tier GET /api/v1/system/status
// uses. A dev build (version.Version == "dev") never reports
// update_available: there is no meaningful "newer" to compare against an
// unreleased build.
func (rt *Router) handleGetUpdates(w http.ResponseWriter, r *http.Request) {
	if rt.demo != nil {
		writeDemoUpdates(w)
		return
	}
	channel := rt.currentUpdateChannel(r.Context())
	resp := updateStatusResource{CurrentVersion: version.Version, ReleaseKind: upgrade.KindRelease, Channel: channel}

	if channel == upgrade.ChannelStable {
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
			resp.UpdateAvailable = upgrade.UpdateAvailable(version.Version, &upgrade.Release{Tag: tag})
			if resp.UpdateAvailable && rt.schemaRefusesRelease(r.Context(), tag, githubAssetNames(release)) {
				resp.UpdateAvailable = false
			}
			rt.warmTrustRoot(resp.UpdateAvailable)
			resp.CheckedAt = rt.updatesCache.checkedAt()
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	release, ok := rt.channelUpdatesCache.Fresh(updatesCacheTTL, channel)
	if !ok {
		fetched, err := rt.upgradeFetchers.LatestForChannel(r.Context(), channel)
		if err != nil {
			rt.logger.Warn("api: fetch latest release for channel failed", slog.String("channel", channel), slog.String("error", err.Error()))
			release, ok = rt.channelUpdatesCache.Stale(channel)
		} else {
			rt.channelUpdatesCache.Set(channel, fetched)
			release, ok = fetched, true
		}
	}
	if ok && release != nil {
		tag, url, published := release.Tag, release.URL, release.PublishedAt
		resp.LatestVersion = &tag
		resp.ReleaseURL = &url
		resp.PublishedAt = &published
		resp.UpdateAvailable = upgrade.UpdateAvailable(version.Version, release)
		if resp.UpdateAvailable && !release.IsEdge() && rt.schemaRefusesRelease(r.Context(), tag, release.AssetNames) {
			resp.UpdateAvailable = false
		}
		rt.warmTrustRoot(resp.UpdateAvailable && !release.IsEdge())
		resp.ReleaseKind = releaseKind(release)
		if release.IsEdge() {
			image := release.Image
			resp.Image = &image
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// currentUpdateChannel reads update_settings.channel, falling back to
// upgrade.ChannelStable on a read error or an unrecognized stored value:
// the same "never fail the whole request over a settings read" stance
// handleUpdatePreflight's own latestRelease already takes on a failed
// GitHub fetch.
func (rt *Router) currentUpdateChannel(ctx context.Context) string {
	settings, err := rt.updateSettings.GetUpdateSettings(ctx)
	if err != nil || !upgrade.ValidChannel(settings.Channel) {
		return upgrade.ChannelStable
	}
	return settings.Channel
}

package api

import (
	"context"
	"strings"
	"sync"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/selfupgrade"
	"github.com/glincker/levelrail/kit/upgrade"
)

// discoverMirror is swapped in tests; production reads the mirror's latest.json.
var discoverMirror = func(ctx context.Context, base, channel string) (selfupgrade.Discovered, error) {
	return selfupgrade.Discoverer{Repo: githubRepo}.FromMirror(ctx, base, channel)
}

// fallbackMirrorBase is the live updates.fallback_release_base_url, empty when unset.
func (rt *Router) fallbackMirrorBase(ctx context.Context) string {
	if rt == nil || rt.runtimeSettings == nil {
		return ""
	}
	return strings.TrimSpace(rt.runtimeSettings.String(ctx, runtimesettings.KeyUpdatesFallbackBaseURL))
}

// mirrorRelease names the channel's newest tag from the mirror listing. Like the
// public release pages it carries no notes, so callers treat it as a fallback.
func mirrorRelease(ctx context.Context, base, channel string) (*upgrade.Release, error) {
	found, err := discoverMirror(ctx, base, channel)
	if err != nil {
		return nil, err
	}
	rel := &upgrade.Release{Tag: found.Tag, URL: found.URL, PublishedAt: found.PublishedAt}
	if channel == upgrade.ChannelEdge {
		sha := strings.TrimPrefix(found.Tag, edgeTagPrefix)
		rel.Kind, rel.Commit, rel.Image = upgrade.KindEdgeImage, sha, upgrade.EdgeImage(githubRepo)
	}
	return rel, nil
}

// channelState remembers, per non-stable channel, whether the last lookup failed
// (the cached list is stale) and which tag came only from the mirror listing.
type channelState struct {
	mu       sync.Mutex
	stale    map[string]bool
	mirrored map[string]string
}

func (s *channelState) record(channel string, stale bool, mirrorTag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stale == nil {
		s.stale, s.mirrored = map[string]bool{}, map[string]string{}
	}
	s.stale[channel], s.mirrored[channel] = stale, mirrorTag
}

func (s *channelState) isStale(channel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stale[channel]
}

func (s *channelState) fromMirror(channel, tag string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return tag != "" && s.mirrored[channel] == tag
}

// channelRelease is the beta and edge lookup: cache, then GitHub, then the
// mirror listing, then the stale cache. A stale answer is flagged so automatic
// updates hold, the same rule stable already follows.
func (rt *Router) channelRelease(ctx context.Context, channel string) (*upgrade.Release, bool) {
	return rt.channelReleaseVia(ctx, channel, func() string { return rt.fallbackMirrorBase(ctx) })
}

func (rt *Router) channelReleaseVia(ctx context.Context, channel string, mirrorBase func() string) (*upgrade.Release, bool) {
	if release, ok := rt.channelUpdatesCache.Fresh(updatesCacheTTL, channel); ok {
		return release, true
	}
	fetched, err := rt.upgradeFetchers.LatestForChannel(ctx, channel)
	if err == nil {
		rt.channelUpdatesCache.Set(channel, fetched)
		rt.channels.record(channel, false, "")
		return fetched, true
	}
	if base := mirrorBase(); base != "" {
		if rel, merr := mirrorRelease(ctx, base, channel); merr == nil {
			rt.channelUpdatesCache.Set(channel, rel)
			rt.channels.record(channel, false, rel.Tag)
			return rel, true
		}
	}
	rt.channels.record(channel, true, "")
	return rt.channelUpdatesCache.Stale(channel)
}

// discoverForChannel is the self-upgrade release list fallback: beta installs
// learn their prerelease from the mirror listing, others the newest stable tag.
func (rt *Router) discoverForChannel(ctx context.Context) (*githubRelease, error) {
	mirror := rt.fallbackMirrorBase(ctx)
	if rt.currentUpdateChannel(ctx) == upgrade.ChannelBeta && mirror != "" {
		if rel, err := mirrorRelease(ctx, mirror, upgrade.ChannelBeta); err == nil {
			return &githubRelease{TagName: rel.Tag, HTMLURL: rel.URL, PublishedAt: rel.PublishedAt, Fallback: true}, nil
		}
	}
	return discoverStableReleaseVia(ctx, mirror)
}

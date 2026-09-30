package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgrade"
)

// VersionSkewSettingsSource is the narrow store surface
// EvaluateVersionSkew needs: the configured update channel
// (migrations/0258_update_settings.sql). *store.DB satisfies this
// structurally.
type VersionSkewSettingsSource interface {
	GetUpdateSettings(ctx context.Context) (store.UpdateSettings, error)
}

// EvaluateVersionSkew runs one KindVersionSkew rule: it fires while
// currentVersion is behind the configured channel's latest release.
// cache is checked before calling fetchers, so a rule enabled alongside
// Engine's own short evaluation interval never hits GitHub more often
// than upgrade.DefaultCacheTTL, the same cadence GET /api/v1/updates and
// updatecheck.Scheduler already hold themselves to.
func EvaluateVersionSkew(ctx context.Context, settings VersionSkewSettingsSource, fetchers upgrade.Fetchers, cache *upgrade.Cache, r Rule, currentVersion string, now time.Time) (Rule, string, error) {
	s, err := settings.GetUpdateSettings(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: get update settings: %w", r.ID, err)
	}
	channel := s.Channel
	if !upgrade.ValidChannel(channel) {
		channel = upgrade.ChannelStable
	}

	release, ok := cache.Fresh(upgrade.DefaultCacheTTL, channel)
	if !ok {
		fetched, ferr := fetchers.LatestForChannel(ctx, channel)
		if ferr != nil {
			if release, ok = cache.Stale(channel); !ok {
				return r, "", fmt.Errorf("alerting: evaluate rule %q: fetch latest release: %w", r.ID, ferr)
			}
		} else {
			cache.Set(channel, fetched)
			release = fetched
		}
	}

	next := r
	next.LastEvaluatedAt = &now

	skewed := upgrade.UpdateAvailable(currentVersion, release)
	var notice string
	if skewed {
		notice = fmt.Sprintf("running %s, %s channel's latest is %s", currentVersion, channel, release.Tag)
	}
	return advanceState(next, r, skewed, 0, now), notice, nil
}

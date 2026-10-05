package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/upgrade"
	"github.com/GLINCKER/levelrail/internal/version"
)

const releaseNotesMaxRunes = 1200

// engineVersioner is implemented by the Docker client behind DockerPinger.
type engineVersioner interface {
	ServerVersion(ctx context.Context) (string, error)
}

type updatePreflightResource struct {
	CurrentVersion  string          `json:"current_version"`
	LatestVersion   *string         `json:"latest_version"`
	UpdateAvailable bool            `json:"update_available"`
	ReleaseURL      *string         `json:"release_url"`
	ReleaseNotes    string          `json:"release_notes"`
	Checks          []upgrade.Check `json:"checks"`
	Blocked         bool            `json:"blocked"`
	UpgradeCommand  string          `json:"upgrade_command"`
	RollbackCommand string          `json:"rollback_command"`
}

// handleUpdatePreflight handles GET /api/v1/updates/preflight: read-only
// checks that gate a self-upgrade, plus the command an operator runs. It never
// changes anything and never upgrades.
func (rt *Router) handleUpdatePreflight(w http.ResponseWriter, r *http.Request) {
	out := updatePreflightResource{
		CurrentVersion:  version.Version,
		UpgradeCommand:  upgradeCommand(""),
		RollbackCommand: "levelrail restore-snapshot --list",
	}
	release, _, known := rt.latestReleaseForCurrentChannel(r.Context())
	var assets []string
	if known && release != nil {
		tag, url := release.Tag, release.URL
		out.LatestVersion, out.ReleaseURL = &tag, &url
		out.UpdateAvailable = upgrade.UpdateAvailable(version.Version, release)
		out.ReleaseNotes = truncateRunes(strings.TrimSpace(release.Body), releaseNotesMaxRunes)
		out.UpgradeCommand = upgradeCommand(tag)
		assets = release.AssetNames
	}
	out.Checks = upgrade.Run(r.Context(), upgrade.Inputs{
		Lookup:        os.LookupEnv,
		DockerVersion: rt.dockerEngineVersion(),
		FreeBytes:     rt.dataDirFreeBytes(),
		NewestBackup:  rt.newestCPBackup(),
		Now:           time.Now(),
		AssetNames:    assets,
		ReleaseKnown:  known && release != nil,
	})
	out.Blocked = upgrade.Blocked(out.Checks)
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) latestRelease(ctx context.Context) (*githubRelease, bool) {
	if release, ok := rt.updatesCache.fresh(updatesCacheTTL); ok {
		return release, true
	}
	fetched, err := rt.fetchLatestRelease(ctx)
	if err != nil {
		return rt.updatesCache.stale()
	}
	rt.updatesCache.set(fetched)
	return fetched, true
}

// latestReleaseForCurrentChannel mirrors handleGetUpdates' channel branch
// (internal/api/updates.go) so preflight checks match whatever channel the
// operator configured instead of always checking stable: before this, an
// operator on beta/edge saw the version card report an upgrade available
// while the preflight card below it said "latest release could not be
// fetched", because it never looked past the stable release list.
func (rt *Router) latestReleaseForCurrentChannel(ctx context.Context) (*upgrade.Release, string, bool) {
	channel := rt.currentUpdateChannel(ctx)
	if channel == upgrade.ChannelStable {
		release, ok := rt.latestRelease(ctx)
		if !ok || release == nil {
			return nil, channel, false
		}
		names := make([]string, 0, len(release.Assets))
		for _, a := range release.Assets {
			names = append(names, a.Name)
		}
		return &upgrade.Release{Tag: release.TagName, URL: release.HTMLURL, PublishedAt: release.PublishedAt, Body: release.Body, AssetNames: names}, channel, true
	}

	release, ok := rt.channelUpdatesCache.Fresh(updatesCacheTTL, channel)
	if !ok {
		fetched, err := rt.upgradeFetchers.LatestForChannel(ctx, channel)
		if err != nil {
			release, ok = rt.channelUpdatesCache.Stale(channel)
		} else {
			rt.channelUpdatesCache.Set(channel, fetched)
			release, ok = fetched, true
		}
	}
	if !ok || release == nil {
		return nil, channel, false
	}
	return release, channel, true
}

func (rt *Router) dockerEngineVersion() func(context.Context) (string, error) {
	v, ok := rt.dockerPinger.(engineVersioner)
	if !ok {
		return nil
	}
	return v.ServerVersion
}

func (rt *Router) dataDirFreeBytes() func() (int64, error) {
	if rt.dataDir == "" {
		return nil
	}
	return func() (int64, error) {
		var st syscall.Statfs_t
		if err := syscall.Statfs(rt.dataDir, &st); err != nil {
			return 0, err
		}
		return int64(st.Bavail) * int64(st.Bsize), nil //nolint:gosec // statfs fields are non-negative
	}
}

func (rt *Router) newestCPBackup() func() (time.Time, bool, error) {
	if rt.cpBackups == nil {
		return nil
	}
	return func() (time.Time, bool, error) {
		list, err := rt.cpBackups.List()
		if err != nil {
			return time.Time{}, false, err
		}
		var newest time.Time
		for _, b := range list {
			if b.CreatedAt.After(newest) {
				newest = b.CreatedAt
			}
		}
		return newest, !newest.IsZero(), nil
	}
}

func upgradeCommand(tag string) string {
	if tag == "" {
		return "curl -fsSL https://raw.githubusercontent.com/" + githubRepo + "/main/install.sh | sudo sh -s upgrade"
	}
	return "curl -fsSL https://raw.githubusercontent.com/" + githubRepo + "/main/install.sh | sudo LEVELRAIL_VERSION=" + tag + " sh -s upgrade"
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

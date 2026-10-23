package api

import (
	"context"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const releaseNotesMaxRunes = 1200

// engineVersioner is implemented by the Docker client behind DockerPinger.
type engineVersioner interface {
	ServerVersion(ctx context.Context) (string, error)
}

type updatePreflightResource struct {
	CurrentVersion     string          `json:"current_version"`
	LatestVersion      *string         `json:"latest_version"`
	UpdateAvailable    bool            `json:"update_available"`
	ReleaseURL         *string         `json:"release_url"`
	ReleaseNotes       string          `json:"release_notes"`
	ReleasePublishedAt string          `json:"release_published_at,omitempty"`
	Checks             []upgrade.Check `json:"checks"`
	Blocked            bool            `json:"blocked"`
	// Waiting is set when every blocker is retryable and expected to clear on
	// its own, such as a release that is still being published.
	Waiting         bool   `json:"waiting"`
	UpgradeCommand  string `json:"upgrade_command"`
	RollbackCommand string `json:"rollback_command"`
	// ReleaseKind is "release" or "edge_image". Edge carries guidance for
	// this install instead of a release upgrade command, and is never
	// applied by Upgrade now, a schedule or the automatic policy.
	ReleaseKind string        `json:"release_kind"`
	Channel     string        `json:"channel"`
	Edge        *edgeGuidance `json:"edge,omitempty"`
	// State classifies this install against the newest release (UpdateState*).
	State               string `json:"state"`
	StateReason         string `json:"state_reason"`
	DBSchema            *int   `json:"db_schema"`
	LatestSchema        *int   `json:"latest_schema"`
	MigrationsToApply   *int   `json:"migrations_to_apply"`
	RestorePointFirst   bool   `json:"restore_point_first"`
	VerifyWindowSeconds int    `json:"verify_window_seconds"`
	// RollbackVersion is the newest retained release a rollback can return to.
	RollbackVersion string `json:"rollback_version,omitempty"`
}

// handleUpdatePreflight handles GET /api/v1/updates/preflight: read-only
// checks that gate a self-upgrade, plus the command an operator runs. It never
// changes anything and never upgrades. refresh=1 bypasses the release cache,
// which is what a Re-check button asks for.
func (rt *Router) handleUpdatePreflight(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		rt.expireReleaseCaches()
	}
	release, known := rt.latestReleaseForCurrentChannel(r.Context())
	writeJSON(w, http.StatusOK, rt.evaluatePreflight(r.Context(), release, known && release != nil))
}

// evaluatePreflight runs the checks against release. The dashboard, the CLI
// and the upgrade scheduler all go through here so they agree on what blocks.
func (rt *Router) evaluatePreflight(ctx context.Context, release *upgrade.Release, known bool) updatePreflightResource {
	out := updatePreflightResource{
		CurrentVersion:  version.Version,
		UpgradeCommand:  upgradeCommand(""),
		RollbackCommand: "levelrail restore-snapshot --list",
		ReleaseKind:     upgrade.KindRelease,
		Channel:         rt.currentUpdateChannel(ctx),
	}
	var assets []string
	var upgradeTag, kind string
	var published time.Time
	if known && release != nil {
		tag, url := release.Tag, release.URL
		out.LatestVersion, out.ReleaseURL = &tag, &url
		out.UpdateAvailable = upgrade.UpdateAvailable(version.Version, release)
		out.ReleaseNotes = truncateRunes(cleanReleaseNotes(release.Body), releaseNotesMaxRunes)
		out.ReleasePublishedAt = release.PublishedAt
		upgradeTag = tag
		assets = release.AssetNames
		if t, err := time.Parse(time.RFC3339, release.PublishedAt); err == nil {
			published = t
		}
		kind = releaseKind(release)
		out.ReleaseKind = kind
		out.Edge = buildEdgeGuidance(release)
	}
	var schema *upgrade.SchemaInput
	if out.UpdateAvailable && kind != upgrade.KindEdgeImage {
		schema = rt.schemaInputFor(ctx, upgradeTag, assets)
	}
	out.Checks = upgrade.Run(ctx, upgrade.Inputs{
		Lookup:             os.LookupEnv,
		DockerVersion:      rt.dockerEngineVersion(),
		FreeBytes:          rt.dataDirFreeBytes(),
		NewestBackup:       rt.newestCPBackup(),
		Now:                time.Now(),
		AssetNames:         assets,
		ReleaseKnown:       known && release != nil,
		ReleasePublishedAt: published,
		ReleaseKind:        kind,
		TrustRoot:          rt.trustRootState(),
		Schema:             schema,
	})
	rt.fillInstallState(ctx, &out, upgradeTag, out.UpdateAvailable, assets)
	if upgrade.SchemaBlocked(out.Checks) {
		out.UpdateAvailable = false
	}
	if rt.dataDir != "" {
		_, pinned, _ := selfupgrade.ReadPin(rt.dataDir)
		out.Checks = append(out.Checks, signaturePinCheck(signaturePinInputs{
			Pinned: pinned, ReleaseKnown: known && release != nil,
			Edge: kind == upgrade.KindEdgeImage, Assets: assets,
			TrustRoot: rt.trustRootState(),
		}))
	}
	out.UpgradeCommand = upgradeCommand(upgradeTag)
	rt.warmTrustRoot(out.UpdateAvailable && kind != upgrade.KindEdgeImage)
	if out.Edge != nil {
		out.UpgradeCommand = out.Edge.RecreateCommand
		if out.Edge.Installable {
			out.UpgradeCommand = out.Edge.PullCommand + " && " + out.Edge.RecreateCommand
		}
		out.RollbackCommand = ""
	}
	out.Blocked = upgrade.Blocked(out.Checks)
	out.Waiting = out.Blocked && onlyRetryableBlockers(out.Checks)
	return out
}

func onlyRetryableBlockers(checks []upgrade.Check) bool {
	found := false
	for _, c := range checks {
		if c.Status != upgrade.StatusFail && c.Status != upgrade.StatusPending {
			continue
		}
		found = true
		if !c.Retryable {
			return false
		}
	}
	return found
}

// expireReleaseCaches makes the next release lookup go to GitHub while
// keeping the last result as a fallback if that lookup fails.
func (rt *Router) expireReleaseCaches() {
	rt.updatesCache.expire()
	rt.channelUpdatesCache.Expire()
	if rt.schemaSource != nil {
		rt.schemaSource.forget()
	}
	if rt.selfUpgrade != nil {
		rt.selfUpgrade.expire()
	}
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
func (rt *Router) latestReleaseForCurrentChannel(ctx context.Context) (*upgrade.Release, bool) {
	channel := rt.currentUpdateChannel(ctx)
	if channel == upgrade.ChannelStable {
		release, ok := rt.latestRelease(ctx)
		if !ok || release == nil {
			return nil, false
		}
		names := make([]string, 0, len(release.Assets))
		for _, a := range release.Assets {
			names = append(names, a.Name)
		}
		return &upgrade.Release{Tag: release.TagName, URL: release.HTMLURL, PublishedAt: release.PublishedAt, Body: release.Body, AssetNames: names}, true
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
		return nil, false
	}
	return release, true
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

// upgradeCommand is the host command that upgrades this install. The server
// verifies signatures itself, so it never asks the installer to require cosign.
func upgradeCommand(tag string) string {
	env := ""
	if tag != "" {
		env += " LEVELRAIL_VERSION=" + tag
	}
	return "curl -fsSL https://raw.githubusercontent.com/" + githubRepo + "/main/install.sh | sudo" + env + " sh -s upgrade"
}

// WarmTrustRoot starts the background Sigstore trust data download at startup.
func (rt *Router) WarmTrustRoot() { rt.warmTrustRoot(true) }

// warmTrustRoot fetches the Sigstore trust data in the background so the
// first upgrade, and later offline ones, can verify the signature.
func (rt *Router) warmTrustRoot(updateAvailable bool) {
	if updateAvailable && rt.trustRoot != nil {
		rt.trustRoot.Warm()
	}
}

func (rt *Router) trustRootState() func() string {
	if rt.trustRoot == nil {
		return nil
	}
	return rt.trustRoot.State
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

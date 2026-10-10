package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/rollback"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/semver"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	releaseHistoryLimit   = 5
	releaseNotesHistoryMx = 3000
	historyFetchTimeout   = 12 * time.Second
	liveDBFile            = "levelrail.db"
	changesBetweenLimit   = 10
)

// releaseHistorySource is the seam tests override instead of calling GitHub.
type releaseHistorySource struct {
	list      func(ctx context.Context, channel string) ([]upgrade.HistoryRelease, error)
	manifest  func(ctx context.Context, url string) (upgrade.Manifest, error)
	mu        sync.Mutex
	cached    map[string]cachedHistory
	manifests map[string]int
}

type cachedHistory struct {
	at       time.Time
	releases []upgrade.HistoryRelease
}

func newReleaseHistorySource(repo string) *releaseHistorySource {
	return &releaseHistorySource{
		list: func(ctx context.Context, channel string) ([]upgrade.HistoryRelease, error) {
			return upgrade.FetchHistory(ctx, repo, channel, releaseHistoryLimit)
		},
		manifest:  upgrade.FetchManifest,
		cached:    map[string]cachedHistory{},
		manifests: map[string]int{},
	}
}

// releases returns the channel's newest releases, serving a stale list when
// GitHub cannot be reached. The bool reports whether the list is fresh.
func (s *releaseHistorySource) releases(ctx context.Context, channel string) ([]upgrade.HistoryRelease, bool, error) {
	s.mu.Lock()
	c, ok := s.cached[channel]
	s.mu.Unlock()
	if ok && time.Since(c.at) < updatesCacheTTL {
		return c.releases, true, nil
	}
	ctx, cancel := context.WithTimeout(ctx, historyFetchTimeout)
	defer cancel()
	list, err := s.list(ctx, channel)
	if err != nil {
		if ok {
			return c.releases, false, nil
		}
		return nil, false, err
	}
	s.mu.Lock()
	s.cached[channel] = cachedHistory{at: time.Now(), releases: list}
	s.mu.Unlock()
	return list, true, nil
}

// schemaFor returns a release's schema version from its manifest asset, -1
// when it has none. Manifests are immutable per tag, so results are kept.
func (s *releaseHistorySource) schemaFor(ctx context.Context, r upgrade.HistoryRelease) int {
	s.mu.Lock()
	v, ok := s.manifests[r.Tag]
	s.mu.Unlock()
	if ok {
		return v
	}
	asset, has := r.Asset(upgrade.ManifestAsset)
	if !has {
		return -1
	}
	m, err := s.manifest(ctx, asset.URL)
	if err != nil || m.SchemaVersion <= 0 {
		return -1
	}
	s.mu.Lock()
	s.manifests[r.Tag] = m.SchemaVersion
	s.mu.Unlock()
	return m.SchemaVersion
}

func (s *releaseHistorySource) prefetchSchemas(ctx context.Context, list []upgrade.HistoryRelease) {
	var wg sync.WaitGroup
	for _, r := range list {
		wg.Add(1)
		go func(r upgrade.HistoryRelease) {
			defer wg.Done()
			s.schemaFor(ctx, r)
		}(r)
	}
	wg.Wait()
}

type releaseHistoryItem struct {
	Version        string                `json:"version"`
	URL            string                `json:"url"`
	PublishedAt    string                `json:"published_at"`
	Channel        string                `json:"channel"`
	Running        bool                  `json:"running"`
	Retained       bool                  `json:"retained"`
	AssetName      string                `json:"asset_name"`
	AssetAvailable bool                  `json:"asset_available"`
	AssetSize      int64                 `json:"asset_size"`
	Signed         bool                  `json:"signed"`
	SchemaVersion  *int                  `json:"schema_version"`
	SchemaSource   string                `json:"schema_source"`
	Verdict        upgrade.SchemaVerdict `json:"verdict"`
	Notes          string                `json:"notes"`
}

type releaseHistoryResource struct {
	CurrentVersion       string               `json:"current_version"`
	CurrentSchemaVersion *int                 `json:"current_schema_version"`
	Channel              string               `json:"channel"`
	View                 string               `json:"view"`
	Reachable            bool                 `json:"github_reachable"`
	Releases             []releaseHistoryItem `json:"releases"`
	RetainedOnly         []releaseHistoryItem `json:"retained_only"`
}

func (rt *Router) assetName() string {
	return rt.brand.BinaryName + "-linux-" + runtime.GOARCH
}

func (rt *Router) retainedDir() (dir, name string) {
	exe, err := os.Executable()
	if err != nil {
		return "", ""
	}
	return rollback.Dir(exe), filepath.Base(exe)
}

func (rt *Router) retainedByVersion() map[string]rollback.Retained {
	out := map[string]rollback.Retained{}
	dir, name := rt.retainedDir()
	if dir == "" {
		return out
	}
	list, err := rollback.List(dir, name)
	if err != nil {
		rt.logger.Warn("api: list retained releases failed", slog.String("error", err.Error()))
		return out
	}
	for _, r := range list {
		out[r.Version] = r
	}
	return out
}

func (rt *Router) liveSchemaVersion(ctx context.Context) int {
	if rt.dataDir == "" {
		return -1
	}
	v, err := rollback.DBSchemaVersion(ctx, filepath.Join(rt.dataDir, liveDBFile))
	if err != nil {
		rt.logger.Warn("api: read database schema version failed", slog.String("error", err.Error()))
		return -1
	}
	return v
}

func optInt(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}

func historyView(q string, configured string) string {
	switch q {
	case upgrade.ChannelStable, upgrade.ChannelBeta, upgrade.ChannelAll:
		return q
	}
	if configured == upgrade.ChannelBeta {
		return upgrade.ChannelBeta
	}
	return upgrade.ChannelStable
}

func (rt *Router) historyItem(ctx context.Context, r upgrade.HistoryRelease, dbSchema int, retained map[string]rollback.Retained) releaseHistoryItem {
	asset, has := r.Asset(rt.assetName())
	ret, isRet := retained[r.Tag]
	schema, source := -1, rollback.SourceUnknown
	switch {
	case isRet && ret.SchemaVersion >= 0:
		schema, source = ret.SchemaVersion, rollback.SourceRetained
	default:
		if v := rt.releaseHist.schemaFor(ctx, r); v >= 0 {
			schema, source = v, rollback.SourceManifest
		}
	}
	channel := upgrade.ChannelStable
	if r.Prerelease {
		channel = upgrade.ChannelBeta
	}
	_, hasSig := r.Asset(upgrade.SignatureAsset)
	return releaseHistoryItem{
		Version: r.Tag, URL: r.URL, PublishedAt: r.PublishedAt, Channel: channel,
		Running: r.Tag == version.Version, Retained: isRet,
		AssetName: rt.assetName(), AssetAvailable: has || isRet, AssetSize: asset.Size, Signed: hasSig,
		SchemaVersion: optInt(schema), SchemaSource: source,
		Verdict: upgrade.VerdictFor(dbSchema, schema),
		Notes:   truncateRunes(cleanReleaseNotes(r.Body), releaseNotesHistoryMx),
	}
}

// handleReleaseHistory handles GET /api/v1/updates/releases: the newest
// releases for a channel with a per-release verdict against the database's
// schema version. Read-only.
func (rt *Router) handleReleaseHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	configured := rt.currentUpdateChannel(ctx)
	view := historyView(r.URL.Query().Get("channel"), configured)
	dbSchema := rt.liveSchemaVersion(ctx)
	retained := rt.retainedByVersion()
	out := releaseHistoryResource{
		CurrentVersion: version.Version, CurrentSchemaVersion: optInt(dbSchema), Channel: configured, View: view,
		Releases: []releaseHistoryItem{}, RetainedOnly: []releaseHistoryItem{},
	}
	list, fresh, err := rt.releaseHist.releases(ctx, view)
	if err != nil {
		rt.logger.Warn("api: fetch release history failed", slog.String("channel", view), slog.String("error", err.Error()))
	}
	out.Reachable = err == nil && fresh
	rt.releaseHist.prefetchSchemas(ctx, list)
	seen := map[string]bool{}
	for _, rel := range list {
		seen[rel.Tag] = true
		out.Releases = append(out.Releases, rt.historyItem(ctx, rel, dbSchema, retained))
	}
	for tag, ret := range retained {
		if seen[tag] {
			continue
		}
		out.RetainedOnly = append(out.RetainedOnly, releaseHistoryItem{
			Version: tag, Channel: channelOfTag(tag), Running: tag == version.Version, Retained: true,
			AssetName: rt.assetName(), AssetAvailable: true, AssetSize: ret.SizeBytes,
			SchemaVersion: optInt(ret.SchemaVersion), SchemaSource: rollbackSource(ret.SchemaVersion),
			Verdict: upgrade.VerdictFor(dbSchema, ret.SchemaVersion),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func rollbackSource(schema int) string {
	if schema >= 0 {
		return rollback.SourceRetained
	}
	return rollback.SourceUnknown
}

func channelOfTag(tag string) string {
	if strings.Contains(tag, "-") {
		return upgrade.ChannelBeta
	}
	return upgrade.ChannelStable
}

type rollbackChange struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

type rollbackPlanResource struct {
	rollback.Plan
	Checks       []upgrade.Check  `json:"checks"`
	Blocked      bool             `json:"blocked"`
	FetchCommand string           `json:"fetch_command,omitempty"`
	Changes      []rollbackChange `json:"changes"`
	Notes        string           `json:"notes"`
}

// handleRollbackPlan handles GET /api/v1/updates/rollback-plan?version=: a
// read-only preview of returning to one release. Root only, because it
// names the database backups on this host. It applies nothing.
func (rt *Router) handleRollbackPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	target := r.URL.Query().Get("version")
	if !rollback.ValidVersion(target) {
		writeError(w, http.StatusBadRequest, "version must be a release tag such as v1.2.3")
		return
	}
	dbSchema := rt.liveSchemaVersion(ctx)
	retained := rt.retainedByVersion()
	ret, isRet := retained[target]

	var rel *upgrade.HistoryRelease
	var all []upgrade.HistoryRelease
	if list, _, err := rt.releaseHist.releases(ctx, upgrade.ChannelAll); err == nil {
		all = list
		for i := range list {
			if list[i].Tag == target {
				rel = &list[i]
			}
		}
	}
	if rel == nil && !isRet {
		writeError(w, http.StatusNotFound, "that release is not among the recent releases or retained on this host")
		return
	}
	schema, source := -1, rollback.SourceUnknown
	if isRet && ret.SchemaVersion >= 0 {
		schema, source = ret.SchemaVersion, rollback.SourceRetained
	} else if rel != nil {
		if v := rt.releaseHist.schemaFor(ctx, *rel); v >= 0 {
			schema, source = v, rollback.SourceManifest
		}
	}

	var dbSize int64
	if rt.dataDir != "" {
		if st, err := os.Stat(filepath.Join(rt.dataDir, liveDBFile)); err == nil {
			dbSize = st.Size()
		}
	}
	var backups []rollback.BackupOption
	if rt.dataDir != "" {
		if b, err := rollback.ListBackups(ctx, rt.dataDir); err == nil {
			backups = b
		}
	}
	plan := rollback.BuildPlan(rollback.PlanInput{
		CurrentVersion: version.Version, CurrentSchemaVersion: dbSchema, DBSizeBytes: dbSize, Backups: backups,
		ProgramName: rt.brand.BinaryName,
		Target:      rollback.Target{Version: target, SchemaVersion: schema, SchemaSource: source, Retained: isRet, SHA256: ret.SHA256},
	})
	out := rollbackPlanResource{Plan: plan, Changes: []rollbackChange{}}
	var assets []string
	if rel != nil {
		assets = rel.AssetNames()
		out.Notes = truncateRunes(cleanReleaseNotes(rel.Body), releaseNotesHistoryMx)
	}
	out.Changes = changesBetween(all, target, version.Version)
	out.Checks = rt.rollbackChecks(ctx, assets, isRet, rel != nil)
	out.Blocked = upgrade.Blocked(out.Checks)
	if !isRet {
		out.FetchCommand = "curl -fsSL https://raw.githubusercontent.com/" + githubRepo + "/main/install.sh | sudo LEVELRAIL_VERSION=" + target + " sh -s retain"
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) rollbackChecks(ctx context.Context, assets []string, retained, known bool) []upgrade.Check {
	checks := upgrade.Run(ctx, upgrade.Inputs{
		Lookup:        os.LookupEnv,
		DockerVersion: rt.dockerEngineVersion(),
		FreeBytes:     rt.dataDirFreeBytes(),
		NewestBackup:  rt.newestCPBackup(),
		Now:           time.Now(),
		AssetNames:    assets,
		ReleaseKnown:  known,
		LookPath:      exec.LookPath,
	})
	if retained {
		for i := range checks {
			if checks[i].Code == "release_signature" {
				checks[i] = upgrade.Check{Code: "retained_binary", Name: "Retained binary checksum", Status: upgrade.StatusOK,
					Message: "kept on this host from a verified install; its checksum is re-checked before it is used"}
			}
		}
	}
	return checks
}

// changesBetween lists the notes of releases newer than target up to and
// including current: what leaving current behind gives up.
func changesBetween(all []upgrade.HistoryRelease, target, current string) []rollbackChange {
	out := []rollbackChange{}
	for _, r := range all {
		above, ok1 := semver.Compare(r.Tag, target)
		upto, ok2 := semver.Compare(r.Tag, current)
		if !ok1 || !ok2 || above <= 0 || upto > 0 {
			continue
		}
		out = append(out, rollbackChange{Version: r.Tag, Notes: truncateRunes(cleanReleaseNotes(r.Body), releaseNotesHistoryMx/2)})
		if len(out) == changesBetweenLimit {
			break
		}
	}
	return out
}

package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/profile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// ServerProfileStore persists the server profile and reads what a move could cost; *store.DB satisfies it.
type ServerProfileStore interface {
	GetServerProfile(ctx context.Context) (store.ServerProfileSetting, error)
	SetServerProfile(ctx context.Context, profile, actor string, now time.Time) error
	CountEnabledPlugins(ctx context.Context) (int, error)
}

type serverProfileState struct {
	store     ServerProfileStore
	lookupEnv func(string) (string, bool)
	diskBytes func() int64
}

// WithServerProfile enables GET/PUT /api/v1/settings/server-profile.
func WithServerProfile(s ServerProfileStore) Option {
	return func(rt *Router) { rt.serverProfile.store = s }
}

type serverProfileInfo struct {
	ID             string `json:"id"`
	Mesh           bool   `json:"mesh"`
	Plugins        bool   `json:"plugins"`
	Experimental   int    `json:"experimental_features"`
	RetentionDays  int    `json:"retention_days"`
	LoopbackAgent  bool   `json:"loopback_agent"`
	LazyLoops      bool   `json:"lazy_loops"`
	Recommendation bool   `json:"posture_recommendations"`
	SBOMScan       bool   `json:"sbom_scan"`
}

type serverProfileResource struct {
	Active          string              `json:"active"`
	Configured      string              `json:"configured"`
	Source          string              `json:"source"`
	EnvVar          string              `json:"env_var"`
	PinnedByEnv     bool                `json:"pinned_by_env"`
	RestartRequired bool                `json:"restart_required"`
	Recommended     string              `json:"recommended,omitempty"`
	Profiles        []serverProfileInfo `json:"profiles"`
	Problems        []profile.Problem   `json:"problems"`
	NeedsConfirm    bool                `json:"needs_confirm"`
	Saved           bool                `json:"saved"`
	UpdatedBy       string              `json:"updated_by,omitempty"`
	UpdatedAt       *time.Time          `json:"updated_at,omitempty"`
	Error           string              `json:"error,omitempty"`
}

type updateServerProfileRequest struct {
	Profile string `json:"profile"`
	Confirm bool   `json:"confirm"`
	DryRun  bool   `json:"dry_run"`
}

func (rt *Router) profileLookup() func(string) (string, bool) {
	if rt.serverProfile.lookupEnv != nil {
		return rt.serverProfile.lookupEnv
	}
	return rt.meshSettingsLookup()
}

func (rt *Router) profileDisk() int64 {
	if rt.serverProfile.diskBytes != nil {
		return rt.serverProfile.diskBytes()
	}
	return diskTotalBytes(rt.dataDir)
}

func profileInfos(disk int64, lookup func(string) (string, bool)) []serverProfileInfo {
	out := make([]serverProfileInfo, 0, 3)
	for _, p := range profile.All() {
		s := profile.For(p)
		plugins := false
		for _, f := range s.Features {
			plugins = plugins || f == experimental.Plugins
		}
		out = append(out, serverProfileInfo{
			ID: string(p), Mesh: s.Mesh, Plugins: plugins, Experimental: len(s.Features),
			RetentionDays: int(profile.Retention(p, disk, lookup) / (24 * time.Hour)), LoopbackAgent: s.LoopbackAgent,
			LazyLoops: s.LazyLoops, Recommendation: s.Recommendations, SBOMScan: s.EnvDefaults["APP_BUILD_ATTEST"] == "true",
		})
	}
	return out
}

func (rt *Router) buildServerProfile(ctx context.Context) (serverProfileResource, error) {
	var saved store.ServerProfileSetting
	if rt.serverProfile.store != nil {
		s, err := rt.serverProfile.store.GetServerProfile(ctx)
		if err != nil {
			return serverProfileResource{}, err
		}
		saved = s
	}
	lookup := rt.profileLookup()
	configured, src := profile.Resolve(lookup, saved.Profile)
	active := profile.Active()
	res := serverProfileResource{
		Active: string(active), Configured: string(configured), Source: src, EnvVar: profile.EnvVar,
		PinnedByEnv: src == profile.SourceEnv, RestartRequired: configured != active,
		Profiles: profileInfos(rt.profileDisk(), lookup), Problems: []profile.Problem{}, Saved: saved.Set, UpdatedBy: saved.UpdatedBy,
	}
	if !saved.UpdatedAt.IsZero() {
		t := saved.UpdatedAt
		res.UpdatedAt = &t
	}
	if total, _, err := telemetry.HostMemoryBytes(); err == nil && total > 0 && total < profile.LowRAMBytes(lookup) && configured != profile.Minimal && !saved.Set {
		res.Recommended = string(profile.Minimal)
	}
	return res, nil
}

func (rt *Router) profileFacts(ctx context.Context, from, to profile.Profile) (profile.Facts, error) {
	var f profile.Facts
	if rt.nodes != nil {
		nodes, err := rt.nodes.ListNodes(ctx)
		if err != nil {
			return f, err
		}
		for _, n := range nodes {
			if rt.localNodeID == "" || n.ID != rt.localNodeID {
				f.RemoteNodes++
			}
		}
	}
	if n, err := rt.serverProfile.store.CountEnabledPlugins(ctx); err == nil {
		f.EnabledPlugins = n
	}
	var savedMesh store.MeshSetting
	if rt.meshSettings.store != nil {
		m, err := rt.meshSettings.store.GetMeshSetting(ctx)
		if err != nil {
			return f, err
		}
		savedMesh = m
	}
	lookup := rt.profileLookup()
	was, _ := network.ResolveEnabledWithProfile(lookup, savedMesh.Enabled, savedMesh.Set, profile.For(from).Mesh)
	will, _ := network.ResolveEnabledWithProfile(lookup, savedMesh.Enabled, savedMesh.Set, profile.For(to).Mesh)
	f.MeshStops = was && !will
	f.Retention = profile.Retention(from, rt.profileDisk(), lookup)
	f.NewRetention = profile.Retention(to, rt.profileDisk(), lookup)
	if _, ok := lookup("APP_METRICS_RETENTION"); ok {
		f.NewRetention = f.Retention
	}
	return f, nil
}

// handleGetServerProfile serves GET /api/v1/settings/server-profile.
func (rt *Router) handleGetServerProfile(w http.ResponseWriter, r *http.Request) {
	res, err := rt.buildServerProfile(r.Context())
	if err != nil {
		rt.internalError(w, "api: get server profile", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetServerProfile serves PUT /api/v1/settings/server-profile. It only
// saves the choice: most subsystems are wired at startup, so a restart applies it,
// and no move deletes rows.
func (rt *Router) handleSetServerProfile(w http.ResponseWriter, r *http.Request) {
	if rt.serverProfile.store == nil {
		writeError(w, http.StatusNotImplemented, "server profile storage is not configured")
		return
	}
	var req updateServerProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	to, ok := profile.Parse(req.Profile)
	if !ok {
		writeError(w, http.StatusBadRequest, "profile must be minimal, standard or suite")
		return
	}
	res, err := rt.buildServerProfile(r.Context())
	if err != nil {
		rt.internalError(w, "api: set server profile", err)
		return
	}
	if res.PinnedByEnv {
		writeError(w, http.StatusConflict, profile.EnvVar+" pins the profile; unset it on the server to change it here")
		return
	}
	from, _ := profile.Parse(res.Configured)
	facts, err := rt.profileFacts(r.Context(), from, to)
	if err != nil {
		rt.internalError(w, "api: server profile facts", err)
		return
	}
	res.Problems = profile.Check(from, to, facts)
	res.NeedsConfirm = profile.NeedsConfirm(res.Problems)
	if profile.Blocked(res.Problems) || (res.NeedsConfirm && !req.Confirm) {
		res.Error = problemSummary(res.Problems)
	}
	if profile.Blocked(res.Problems) {
		writeJSON(w, http.StatusConflict, res)
		return
	}
	if res.NeedsConfirm && !req.Confirm {
		writeJSON(w, http.StatusConflict, res)
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, res)
		return
	}
	actor := rt.eventActor(r)
	if err := rt.serverProfile.store.SetServerProfile(r.Context(), string(to), actor, time.Now()); err != nil {
		rt.internalError(w, "api: save server profile", err)
		return
	}
	rt.logger.Info("server profile saved", slog.String("profile", string(to)), slog.String("from", string(from)), slog.String("actor", actor))
	res.Configured, res.Source, res.Saved, res.UpdatedBy = string(to), profile.SourceSetting, true, actor
	res.RestartRequired = res.Configured != res.Active
	res.Recommended = ""
	writeJSON(w, http.StatusOK, res)
}

func diskTotalBytes(dir string) int64 {
	var st syscall.Statfs_t
	if dir == "" || syscall.Statfs(dir, &st) != nil {
		return 0
	}
	return int64(st.Blocks) * int64(st.Bsize) //nolint:gosec // block counts of a real filesystem fit int64
}

func problemSummary(ps []profile.Problem) string {
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, p.Message)
	}
	return strings.Join(parts, " ")
}

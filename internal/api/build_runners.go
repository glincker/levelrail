package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Setting keys and env seeds for build runner routing.
const (
	KeyBuildRunnerStrategy   = "build.runner_strategy"
	KeyBuildRunnerStaleAfter = "build.runner_stale_after"
	KeyBuildRemotePressure   = "build.remote_pressure_hold"
	KeyBuildRunnerMinDisk    = runtimesettings.KeyBuildRunnerMinDiskMB

	envBuildRunnerStrategy   = "APP_BUILD_RUNNER_STRATEGY"
	envBuildRunnerStaleAfter = "APP_BUILD_RUNNER_STALE_AFTER"
	envBuildRemotePressure   = "APP_BUILD_REMOTE_PRESSURE_HOLD"

	buildRunnerMaxLabels = 16
	buildRunnerLabelMax  = 128
	buildRunnerCacheWait = 2 * time.Second
)

// Registered here so internal/build does not depend on the settings package.
func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildRunnerStrategy, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindString,
		Default: string(build.StrategyFirst), Env: envBuildRunnerStrategy, Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{string(build.StrategyFirst), string(build.StrategyLeastLoaded), string(build.StrategySticky)},
		Title:          "Build runner choice",
		Description:    "How a build picks among build nodes. first keeps the node with the smallest ID, least_loaded prefers free slots and low host pressure, sticky_per_app also prefers the runner that built the app last. A build always falls back to the control plane when no runner qualifies. Read on every build; the environment variable only seeds the default.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildRunnerStaleAfter, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindDuration,
		Default: "2m", Env: envBuildRunnerStaleAfter, Apply: runtimesettings.ApplyLive,
		CheckValue:  checkRunnerStaleAfter,
		Title:       "Build runner stale after",
		Description: "How old a build node's last heartbeat may be before least_loaded and sticky_per_app skip it. 0 turns the check off. Read on every build.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildRemotePressure, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindBool,
		Default: "true", Env: envBuildRemotePressure, Apply: runtimesettings.ApplyLive,
		Title:       "Skip runners under critical pressure",
		Description: "When on, least_loaded and sticky_per_app never send a build to a node reporting critical host pressure. The first strategy is unchanged. Read on every build.",
	})
}

// wireBuildRunnerSettings feeds the saved values to the build router so changes apply live.
func wireBuildRunnerSettings(svc *runtimesettings.Service) {
	build.SetRunnerPolicyResolver(func(ctx context.Context) build.Policy {
		return build.Policy{
			Strategy:     build.ParseStrategy(svc.String(ctx, KeyBuildRunnerStrategy)),
			PressureHold: svc.Bool(ctx, KeyBuildRemotePressure),
			StaleAfter:   svc.Duration(ctx, KeyBuildRunnerStaleAfter),

			MinDiskFreeBytes: int64(svc.Int(ctx, KeyBuildRunnerMinDisk)) << 20,

			PersonalInProduction: svc.Bool(ctx, KeyBuildPersonalInProduction),
		}
	})
}

// BuildRunnerPrefStore is the per app runner preference store. *store.DB satisfies it.
type BuildRunnerPrefStore interface {
	GetBuildRunnerPref(ctx context.Context, app string) (store.BuildRunnerPref, bool, error)
	SetBuildRunnerPref(ctx context.Context, p store.BuildRunnerPref, now time.Time) error
	ClearBuildRunnerPref(ctx context.Context, app string) error
}

// SetBuildRunnerNodes wires the node source behind the runner list.
func (rt *Router) SetBuildRunnerNodes(src build.NodeSource) { rt.buildRunnerNodes = src }

type buildRunnerResource struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Health        string     `json:"health"`
	SlotsUsed     int        `json:"slots_used"`
	SlotsTotal    int        `json:"slots_total"`
	Waiting       int        `json:"waiting"`
	ActiveBuilds  int        `json:"active_builds"`
	Load          float64    `json:"load"`
	Pressure      string     `json:"pressure"`
	Architecture  string     `json:"architecture,omitempty"`
	CPUCores      int        `json:"cpu_cores,omitempty"`
	Labels        []string   `json:"labels"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	LastBuildAt   *time.Time `json:"last_build_at,omitempty"`
	LastBuildApp  string     `json:"last_build_app,omitempty"`
	LastBuildStat string     `json:"last_build_status,omitempty"`
	CacheBytes    *int64     `json:"cache_bytes,omitempty"`
	Trust         string     `json:"trust,omitempty"`
	// SkipReason is why routing currently skips this runner, empty when it is usable.
	SkipReason string `json:"skip_reason,omitempty"`
}

type buildRunnersResponse struct {
	Strategy string                `json:"strategy"`
	Runners  []buildRunnerResource `json:"runners"`
}

func runnerStatus(n build.NodeInfo) (status, health string) {
	switch {
	case n.Draining:
		return "draining", "draining"
	case !n.Local && !n.Online:
		return "offline", "offline"
	case n.Pressure == build.PressureCritical:
		return "pressure", "degraded"
	case n.SlotsTotal > 0 && n.SlotsUsed >= n.SlotsTotal:
		return "busy", healthFor(n)
	}
	return "ready", healthFor(n)
}

func healthFor(n build.NodeInfo) string {
	if n.Pressure == build.PressureWarn {
		return "degraded"
	}
	return "healthy"
}

func labelStrings(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v == "" {
			out = append(out, k)
		} else {
			out = append(out, k+"="+v)
		}
	}
	sort.Strings(out)
	return out
}

func checkRunnerStaleAfter(canonical string) error {
	d, err := time.ParseDuration(canonical)
	if err != nil || d < 0 || d > 24*time.Hour {
		return fmt.Errorf("value must be a duration from 0 to 24h")
	}
	return nil
}

// buildRunnerList lists the control plane and every build node, with slot usage applied.
func (rt *Router) buildRunnerList(ctx context.Context) []build.NodeInfo {
	var nodes []build.NodeInfo
	if rt.buildRunnerNodes != nil {
		if got, err := rt.buildRunnerNodes(ctx); err == nil {
			nodes = got
		} else {
			rt.logger.Warn("api: list build runners failed", "error", err.Error())
		}
	}
	hasLocal := false
	for _, n := range nodes {
		hasLocal = hasLocal || n.Local
	}
	if !hasLocal {
		nodes = append([]build.NodeInfo{{Name: "control plane", Local: true, Online: true, AcceptsBuildWorkloads: true,
			Arch: build.HostPlatform(), Pressure: build.LocalPressure()}}, nodes...)
	}
	var loads []build.NodeLoad
	if rt.buildLoad != nil {
		loads = rt.buildLoad.Load()
	}
	total := rt.buildLimitsView(ctx).Effective.MaxConcurrent
	runners := make([]build.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.Local || n.AcceptsBuildWorkloads {
			runners = append(runners, n)
		}
	}
	return build.ApplyLoad(runners, loads, total)
}

// handleListBuildRunners handles GET /api/v1/builds/runners.
func (rt *Router) handleListBuildRunners(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodes := rt.buildRunnerList(ctx)
	resp := buildRunnersResponse{Strategy: string(build.StrategyFirst), Runners: make([]buildRunnerResource, 0, len(nodes))}
	if rs := rt.RuntimeSettings(); rs != nil {
		resp.Strategy = string(build.ParseStrategy(rs.String(ctx, KeyBuildRunnerStrategy)))
	}
	type lastRun struct {
		at     time.Time
		app    string
		status string
	}
	last := map[string]lastRun{}
	if rt.buildPerf != nil {
		if runs, err := rt.buildPerf.ListAllBuildRuns(ctx, buildCenterMaxLimit); err == nil {
			for _, run := range runs {
				if _, seen := last[run.NodeID]; !seen {
					last[run.NodeID] = lastRun{at: run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond), app: run.App, status: run.Status}
				}
			}
		}
	}
	policy := build.Policy{Now: time.Now()}
	if rs := rt.RuntimeSettings(); rs != nil {
		policy.StaleAfter = rs.Duration(ctx, KeyBuildRunnerStaleAfter)
		policy.MinDiskFreeBytes = int64(rs.Int(ctx, KeyBuildRunnerMinDisk)) << 20
	}
	for _, n := range nodes {
		status, health := runnerStatus(n)
		res := buildRunnerResource{
			ID: n.ID, Name: n.Name, Kind: "node", Status: status, Health: health,
			SlotsUsed: n.SlotsUsed, SlotsTotal: n.SlotsTotal, Waiting: n.Waiting, ActiveBuilds: n.SlotsUsed,
			Pressure: n.Pressure, Architecture: n.Arch, CPUCores: n.CPUCores, Labels: labelStrings(n.Labels),
			SkipReason: n.SkipReason(policy),
		}
		if !n.Local {
			res.Trust = n.Trust
			if res.Trust == "" {
				res.Trust = build.TrustServer
			}
		}
		if res.Name == "" {
			res.Name = n.ID
		}
		if n.Local {
			res.Kind = "control_plane"
			res.CacheBytes = rt.localBuildCacheBytes(ctx)
		}
		if res.Pressure == "" {
			res.Pressure = build.PressureUnknown
		}
		if n.SlotsTotal > 0 {
			res.Load = float64(n.SlotsUsed) / float64(n.SlotsTotal)
		}
		if !n.LastSeen.IsZero() {
			t := n.LastSeen
			res.LastSeenAt = &t
		}
		if l, ok := last[n.ID]; ok {
			t := l.at
			res.LastBuildAt, res.LastBuildApp, res.LastBuildStat = &t, l.app, l.status
		}
		resp.Runners = append(resp.Runners, res)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) localBuildCacheBytes(ctx context.Context) *int64 {
	if rt.buildCacheNodes == nil {
		return nil
	}
	admin, err := rt.buildCacheNodes("")
	if err != nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, buildRunnerCacheWait)
	defer cancel()
	records, err := admin.BuildCacheRecords(cctx)
	if err != nil {
		return nil
	}
	var total int64
	for _, rec := range records {
		total += rec.SizeBytes
	}
	return &total
}

type appBuildRunnerResource struct {
	App    string   `json:"app"`
	Mode   string   `json:"mode"`
	NodeID string   `json:"node_id"`
	Pinned bool     `json:"pinned"`
	Labels []string `json:"labels"`
}

type setAppBuildRunnerRequest struct {
	Mode   string   `json:"mode"`
	NodeID string   `json:"node_id"`
	Pinned bool     `json:"pinned"`
	Labels []string `json:"labels"`
}

func (rt *Router) buildRunnerPrefs(w http.ResponseWriter) (BuildRunnerPrefStore, bool) {
	s, ok := rt.buildPerf.(BuildRunnerPrefStore)
	if !ok || s == nil {
		writeError(w, http.StatusNotImplemented, "build runner preferences are not available on this control plane")
		return nil, false
	}
	return s, true
}

// handleGetAppBuildRunner handles GET /api/v1/apps/{name}/build-runner.
func (rt *Router) handleGetAppBuildRunner(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.buildRunnerPrefs(w)
	if !ok {
		return
	}
	app := r.PathValue("name")
	p, _, err := s.GetBuildRunnerPref(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: get build runner failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toAppBuildRunner(app, p))
}

func toAppBuildRunner(app string, p store.BuildRunnerPref) appBuildRunnerResource {
	labels := p.Labels
	if labels == nil {
		labels = []string{}
	}
	mode := p.Mode
	if mode == "" {
		mode = build.ModeAuto
	}
	return appBuildRunnerResource{App: app, Mode: mode, NodeID: p.NodeID, Pinned: p.Pinned, Labels: labels}
}

func validRunnerLabel(l string) bool {
	return l != "" && len(l) <= buildRunnerLabelMax && !strings.ContainsAny(l, ", \t\n") && !strings.HasPrefix(l, "=")
}

// validateRunnerMode returns the reason a mode and node combination is invalid, empty when it is fine.
func validateRunnerMode(req setAppBuildRunnerRequest) string {
	switch req.Mode {
	case build.ModeAuto:
		if req.Pinned && req.NodeID == "" {
			return "pinning needs a node_id"
		}
	case build.ModeControlPlane:
		if req.NodeID != "" || req.Pinned {
			return "control_plane mode takes no node_id and no pin"
		}
	case build.ModeNode:
		if req.NodeID == "" {
			return "node mode needs a node_id"
		}
	default:
		return "mode must be auto, control_plane or node"
	}
	return ""
}

// handleSetAppBuildRunner handles PUT /api/v1/apps/{name}/build-runner.
func (rt *Router) handleSetAppBuildRunner(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.buildRunnerPrefs(w)
	if !ok {
		return
	}
	app := r.PathValue("name")
	var req setAppBuildRunnerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Labels) > buildRunnerMaxLabels {
		writeError(w, http.StatusBadRequest, "too many labels")
		return
	}
	for _, l := range req.Labels {
		if !validRunnerLabel(l) {
			writeError(w, http.StatusBadRequest, "labels are key or key=value without spaces or commas")
			return
		}
	}
	if req.Mode == "" {
		req.Mode = build.ModeAuto
	}
	if msg := validateRunnerMode(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.NodeID != "" {
		// Resolve a name to its ID first so a policy on node:<id> cannot be sidestepped.
		id, found := rt.buildRunnerNodeID(r.Context(), req.NodeID)
		req.NodeID = id
		if !found {
			writeError(w, http.StatusNotFound, "node not found")
			return
		}
		if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "node not found"), nodeRef(id)) {
			return
		}
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), app); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set build runner: load app failed", err)
		return
	}
	p := store.BuildRunnerPref{App: app, Mode: req.Mode, NodeID: req.NodeID, Pinned: req.Pinned, Labels: req.Labels}
	if err := s.SetBuildRunnerPref(r.Context(), p, time.Now()); err != nil {
		rt.internalError(w, "api: set build runner failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toAppBuildRunner(app, p))
}

// handleClearAppBuildRunner handles DELETE /api/v1/apps/{name}/build-runner.
func (rt *Router) handleClearAppBuildRunner(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.buildRunnerPrefs(w)
	if !ok {
		return
	}
	app := r.PathValue("name")
	if err := s.ClearBuildRunnerPref(r.Context(), app); err != nil {
		rt.internalError(w, "api: clear build runner failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toAppBuildRunner(app, store.BuildRunnerPref{}))
}

// buildRunnerNodeID resolves a node ID or name to its ID. The control plane never qualifies.
func (rt *Router) buildRunnerNodeID(ctx context.Context, ref string) (string, bool) {
	for _, n := range rt.buildRunnerList(ctx) {
		if !n.Local && (n.ID == ref || n.Name == ref) {
			return n.ID, true
		}
	}
	return "", false
}

// nameBuildRunners adds the runner's display name to rows that ran on a build run.
func (rt *Router) nameBuildRunners(ctx context.Context, rows []buildRow) {
	names := map[string]string{"": "control plane"}
	for _, n := range rt.buildRunnerList(ctx) {
		if n.ID != "" && n.Name != "" {
			names[n.ID] = n.Name
		}
	}
	for i := range rows {
		if rows[i].Platform == "" {
			continue
		}
		if name, ok := names[rows[i].Node]; ok {
			rows[i].RunnerName = name
		} else {
			rows[i].RunnerName = rows[i].Node
		}
	}
}

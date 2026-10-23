package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

const defaultBuildTrendWindow = 20

// BuildPerformanceStore is the persistence behind build limits and build stats.
type BuildPerformanceStore interface {
	RecordBuildRun(ctx context.Context, r store.BuildRun) error
	ListBuildRuns(ctx context.Context, app string, limit int) ([]store.BuildRun, error)
	ListAllBuildRuns(ctx context.Context, limit int) ([]store.BuildRun, error)
	ListBuildAttempts(ctx context.Context, limit int) ([]store.DeployAttempt, error)
	GetBuildLimitSetting(ctx context.Context, key string) (int64, bool, error)
	SetBuildLimitSetting(ctx context.Context, key string, value int64, actor string, now time.Time) error
	ClearBuildLimitSetting(ctx context.Context, key string) error
}

// WithBuildPerformance enables the build stats and build limits routes.
func WithBuildPerformance(s BuildPerformanceStore) Option {
	return func(rt *Router) { rt.buildPerf = s }
}

// BuildRunRecorder adapts the store to the build limiter's recorder.
func BuildRunRecorder(s BuildPerformanceStore) build.RunRecorder { return runRecorder{s} }

type runRecorder struct{ s BuildPerformanceStore }

func (r runRecorder) RecordBuildRun(ctx context.Context, run build.Run) error {
	return r.s.RecordBuildRun(ctx, store.BuildRun{
		App: run.App, Tag: run.Tag, StartedAt: run.StartedAt,
		DurationMS: run.Duration.Milliseconds(), QueueWaitMS: run.QueueWait.Milliseconds(),
		Status: string(run.Status), CachedSteps: run.CachedSteps, TotalSteps: run.TotalSteps, Error: run.Error,
		NodeID: run.Node, Platform: run.Platform, ImageBytes: run.ImageBytes,
	})
}

// BuildLimitsSource resolves saved overrides over the env defaults on every build.
func BuildLimitsSource(s BuildPerformanceStore, env build.Limits) build.LimitsSource {
	return func(ctx context.Context, app string) build.Limits {
		eff, _ := resolveBuildLimits(ctx, s, env)
		eff, _ = resolveAppBuildLimits(ctx, s, eff, app)
		return eff
	}
}

func resolveBuildLimits(ctx context.Context, s BuildPerformanceStore, env build.Limits) (build.Limits, buildLimitOverrides) {
	return resolveScopedBuildLimits(ctx, s, env, func(k string) string { return k })
}

// resolveAppBuildLimits layers app's overrides over base. Concurrency is a
// control-plane wide slot count, so it is never overridden per app.
func resolveAppBuildLimits(ctx context.Context, s BuildPerformanceStore, base build.Limits, app string) (build.Limits, buildLimitOverrides) {
	eff, ov := resolveScopedBuildLimits(ctx, s, base, func(k string) string { return store.BuildLimitAppKey(app, k) })
	eff.MaxConcurrent, ov.MaxConcurrent = base.MaxConcurrent, nil
	return eff, ov
}

func resolveScopedBuildLimits(ctx context.Context, s BuildPerformanceStore, base build.Limits, key func(string) string) (build.Limits, buildLimitOverrides) {
	eff := base
	var ov buildLimitOverrides
	if s == nil {
		return eff, ov
	}
	get := func(k string) (int, bool) {
		v, ok, err := s.GetBuildLimitSetting(ctx, key(k))
		if err != nil || !ok || v < 0 {
			return 0, false
		}
		return int(v), true
	}
	if n, ok := get(store.BuildLimitMaxConcurrent); ok {
		eff.MaxConcurrent, ov.MaxConcurrent = n, &n
	}
	if n, ok := get(store.BuildLimitTimeoutSeconds); ok {
		eff.Timeout, ov.TimeoutSeconds = time.Duration(n)*time.Second, &n
	}
	if n, ok := get(store.BuildLimitMemoryMB); ok {
		eff.MemoryMB, ov.MemoryMB = n, &n
	}
	if n, ok := get(store.BuildLimitCPUMillis); ok {
		eff.CPUMillis, ov.CPUMillis = n, &n
	}
	return eff, ov
}

type buildLimitOverrides struct {
	MaxConcurrent  *int `json:"max_concurrent,omitempty"`
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
	MemoryMB       *int `json:"memory_mb,omitempty"`
	CPUMillis      *int `json:"cpu_millis,omitempty"`
}

type buildLimitValues struct {
	MaxConcurrent  int `json:"max_concurrent"`
	TimeoutSeconds int `json:"timeout_seconds"`
	MemoryMB       int `json:"memory_mb"`
	CPUMillis      int `json:"cpu_millis"`
}

const (
	maxBuildMemoryMB  = 1 << 20
	maxBuildCPUMillis = 256 * 1000
	maxBuildTimeoutS  = 7 * 24 * 3600
)

// valid reports whether every set override is in range; 0 means unlimited.
func (o buildLimitOverrides) valid() bool {
	in := func(p *int, hi int) bool { return p == nil || (*p >= 0 && *p <= hi) }
	return in(o.MaxConcurrent, 64) && in(o.TimeoutSeconds, maxBuildTimeoutS) && in(o.MemoryMB, maxBuildMemoryMB) && in(o.CPUMillis, maxBuildCPUMillis)
}

func (o buildLimitOverrides) empty() bool {
	return o.MaxConcurrent == nil && o.TimeoutSeconds == nil && o.MemoryMB == nil && o.CPUMillis == nil
}

func (o buildLimitOverrides) entries(key func(string) string) map[string]*int {
	return map[string]*int{
		key(store.BuildLimitMaxConcurrent):  o.MaxConcurrent,
		key(store.BuildLimitTimeoutSeconds): o.TimeoutSeconds,
		key(store.BuildLimitMemoryMB):       o.MemoryMB,
		key(store.BuildLimitCPUMillis):      o.CPUMillis,
	}
}

var buildLimitKeys = []string{store.BuildLimitMaxConcurrent, store.BuildLimitTimeoutSeconds, store.BuildLimitMemoryMB, store.BuildLimitCPUMillis}

type buildLimitsResponse struct {
	Effective buildLimitValues    `json:"effective"`
	Default   buildLimitValues    `json:"default"`
	Overrides buildLimitOverrides `json:"overrides"`
}

func toLimitValues(l build.Limits) buildLimitValues {
	return buildLimitValues{MaxConcurrent: l.MaxConcurrent, TimeoutSeconds: int(l.Timeout.Seconds()), MemoryMB: l.MemoryMB, CPUMillis: l.CPUMillis}
}

type buildStatsRun struct {
	StartedAt       time.Time `json:"started_at"`
	Tag             string    `json:"tag"`
	Status          string    `json:"status"`
	DurationSeconds float64   `json:"duration_seconds"`
	QueueWaitSecs   float64   `json:"queue_wait_seconds"`
	CachedSteps     int       `json:"cached_steps"`
	TotalSteps      int       `json:"total_steps"`
	Error           string    `json:"error,omitempty"`
}

type buildStatsResponse struct {
	App   string          `json:"app"`
	Limit int             `json:"limit"`
	Trend build.Trend     `json:"trend"`
	Runs  []buildStatsRun `json:"runs"`
}

func (rt *Router) handleBuildStats(w http.ResponseWriter, r *http.Request) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build stats are not available on this control plane")
		return
	}
	name := r.PathValue("name")
	limit := defaultBuildTrendWindow
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: build stats: load app failed", err)
		return
	}
	rows, err := rt.buildPerf.ListBuildRuns(r.Context(), name, limit)
	if err != nil {
		rt.internalError(w, "api: build stats: list runs failed", err)
		return
	}
	runs := make([]build.Run, 0, len(rows))
	out := buildStatsResponse{App: name, Limit: limit, Runs: make([]buildStatsRun, 0, len(rows))}
	for _, row := range rows {
		runs = append(runs, build.Run{
			Status: build.RunStatus(row.Status), Duration: time.Duration(row.DurationMS) * time.Millisecond,
			CachedSteps: row.CachedSteps, TotalSteps: row.TotalSteps,
		})
		out.Runs = append(out.Runs, buildStatsRun{
			StartedAt: row.StartedAt, Tag: row.Tag, Status: row.Status,
			DurationSeconds: float64(row.DurationMS) / 1000, QueueWaitSecs: float64(row.QueueWaitMS) / 1000,
			CachedSteps: row.CachedSteps, TotalSteps: row.TotalSteps, Error: row.Error,
		})
	}
	out.Trend = build.ComputeTrend(runs)
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) buildLimitsView(ctx context.Context) buildLimitsResponse {
	env := build.LimitsFromEnv(os.LookupEnv)
	eff, ov := resolveBuildLimits(ctx, rt.buildPerf, env)
	return buildLimitsResponse{Effective: toLimitValues(eff), Default: toLimitValues(env), Overrides: ov}
}

func (rt *Router) handleGetBuildLimits(w http.ResponseWriter, r *http.Request) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build limits are not available on this control plane")
		return
	}
	writeJSON(w, http.StatusOK, rt.buildLimitsView(r.Context()))
}

func (rt *Router) handleSetBuildLimits(w http.ResponseWriter, r *http.Request) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build limits are not available on this control plane")
		return
	}
	var req buildLimitOverrides
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.empty() {
		writeError(w, http.StatusBadRequest, "set at least one of max_concurrent, timeout_seconds, memory_mb, cpu_millis (0 means unlimited)")
		return
	}
	if !req.valid() {
		writeError(w, http.StatusBadRequest, "max_concurrent must be 0 to 64, timeout_seconds 0 to 604800, memory_mb 0 to 1048576, cpu_millis 0 to 256000")
		return
	}
	if !rt.storeBuildLimits(w, r, req, func(k string) string { return k }) {
		return
	}
	writeJSON(w, http.StatusOK, rt.buildLimitsView(r.Context()))
}

func (rt *Router) handleResetBuildLimits(w http.ResponseWriter, r *http.Request) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build limits are not available on this control plane")
		return
	}
	for _, k := range buildLimitKeys {
		if err := rt.buildPerf.ClearBuildLimitSetting(r.Context(), k); err != nil {
			rt.internalError(w, "api: reset build limits failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, rt.buildLimitsView(r.Context()))
}

func (rt *Router) storeBuildLimits(w http.ResponseWriter, r *http.Request, req buildLimitOverrides, key func(string) string) bool {
	_, _, actor, _ := rt.currentActor(r)
	now := time.Now()
	for k, v := range req.entries(key) {
		if v == nil {
			continue
		}
		if err := rt.buildPerf.SetBuildLimitSetting(r.Context(), k, int64(*v), actor, now); err != nil {
			rt.internalError(w, "api: set build limit failed", err)
			return false
		}
	}
	return true
}

type appBuildLimitsResponse struct {
	Effective buildLimitValues    `json:"effective"`
	Global    buildLimitValues    `json:"global"`
	Overrides buildLimitOverrides `json:"overrides"`
}

func (rt *Router) appBuildLimitsView(ctx context.Context, app string) appBuildLimitsResponse {
	global, _ := resolveBuildLimits(ctx, rt.buildPerf, build.LimitsFromEnv(os.LookupEnv))
	eff, ov := resolveAppBuildLimits(ctx, rt.buildPerf, global, app)
	return appBuildLimitsResponse{Effective: toLimitValues(eff), Global: toLimitValues(global), Overrides: ov}
}

// appForBuildLimits answers 501 or 404 itself and returns false when the request cannot proceed.
func (rt *Router) appForBuildLimits(w http.ResponseWriter, r *http.Request) (string, bool) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build limits are not available on this control plane")
		return "", false
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: build limits: load app failed", err)
		return "", false
	}
	return name, true
}

func (rt *Router) handleGetAppBuildLimits(w http.ResponseWriter, r *http.Request) {
	if name, ok := rt.appForBuildLimits(w, r); ok {
		writeJSON(w, http.StatusOK, rt.appBuildLimitsView(r.Context(), name))
	}
}

func (rt *Router) handleSetAppBuildLimits(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.appForBuildLimits(w, r)
	if !ok {
		return
	}
	var req buildLimitOverrides
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.MaxConcurrent = nil
	if req.empty() {
		writeError(w, http.StatusBadRequest, "set at least one of timeout_seconds, memory_mb, cpu_millis (0 means unlimited)")
		return
	}
	if !req.valid() {
		writeError(w, http.StatusBadRequest, "timeout_seconds must be 0 to 604800, memory_mb 0 to 1048576, cpu_millis 0 to 256000")
		return
	}
	if !rt.storeBuildLimits(w, r, req, func(k string) string { return store.BuildLimitAppKey(name, k) }) {
		return
	}
	writeJSON(w, http.StatusOK, rt.appBuildLimitsView(r.Context(), name))
}

func (rt *Router) handleResetAppBuildLimits(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.appForBuildLimits(w, r)
	if !ok {
		return
	}
	for _, k := range buildLimitKeys {
		if err := rt.buildPerf.ClearBuildLimitSetting(r.Context(), store.BuildLimitAppKey(name, k)); err != nil {
			rt.internalError(w, "api: reset app build limits failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, rt.appBuildLimitsView(r.Context(), name))
}

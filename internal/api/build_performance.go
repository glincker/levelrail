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
	})
}

// BuildLimitsSource resolves saved overrides over the env defaults on every build.
func BuildLimitsSource(s BuildPerformanceStore, env build.Limits) build.LimitsSource {
	return func(ctx context.Context) build.Limits {
		eff, _ := resolveBuildLimits(ctx, s, env)
		return eff
	}
}

func resolveBuildLimits(ctx context.Context, s BuildPerformanceStore, env build.Limits) (build.Limits, buildLimitOverrides) {
	eff := env
	var ov buildLimitOverrides
	if s == nil {
		return eff, ov
	}
	if v, ok, err := s.GetBuildLimitSetting(ctx, store.BuildLimitMaxConcurrent); err == nil && ok && v >= 0 {
		eff.MaxConcurrent = int(v)
		n := int(v)
		ov.MaxConcurrent = &n
	}
	if v, ok, err := s.GetBuildLimitSetting(ctx, store.BuildLimitTimeoutSeconds); err == nil && ok && v >= 0 {
		eff.Timeout = time.Duration(v) * time.Second
		n := int(v)
		ov.TimeoutSeconds = &n
	}
	return eff, ov
}

type buildLimitOverrides struct {
	MaxConcurrent  *int `json:"max_concurrent,omitempty"`
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
}

type buildLimitValues struct {
	MaxConcurrent  int `json:"max_concurrent"`
	TimeoutSeconds int `json:"timeout_seconds"`
}

type buildLimitsResponse struct {
	Effective buildLimitValues    `json:"effective"`
	Default   buildLimitValues    `json:"default"`
	Overrides buildLimitOverrides `json:"overrides"`
}

func toLimitValues(l build.Limits) buildLimitValues {
	return buildLimitValues{MaxConcurrent: l.MaxConcurrent, TimeoutSeconds: int(l.Timeout.Seconds())}
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
	if req.MaxConcurrent == nil && req.TimeoutSeconds == nil {
		writeError(w, http.StatusBadRequest, "set max_concurrent, timeout_seconds or both (0 means unlimited)")
		return
	}
	if (req.MaxConcurrent != nil && (*req.MaxConcurrent < 0 || *req.MaxConcurrent > 64)) ||
		(req.TimeoutSeconds != nil && (*req.TimeoutSeconds < 0 || *req.TimeoutSeconds > 7*24*3600)) {
		writeError(w, http.StatusBadRequest, "max_concurrent must be 0 to 64 and timeout_seconds 0 to 604800")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	now := time.Now()
	if req.MaxConcurrent != nil {
		if err := rt.buildPerf.SetBuildLimitSetting(r.Context(), store.BuildLimitMaxConcurrent, int64(*req.MaxConcurrent), actor, now); err != nil {
			rt.internalError(w, "api: set build limit failed", err)
			return
		}
	}
	if req.TimeoutSeconds != nil {
		if err := rt.buildPerf.SetBuildLimitSetting(r.Context(), store.BuildLimitTimeoutSeconds, int64(*req.TimeoutSeconds), actor, now); err != nil {
			rt.internalError(w, "api: set build limit failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, rt.buildLimitsView(r.Context()))
}

func (rt *Router) handleResetBuildLimits(w http.ResponseWriter, r *http.Request) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build limits are not available on this control plane")
		return
	}
	for _, k := range []string{store.BuildLimitMaxConcurrent, store.BuildLimitTimeoutSeconds} {
		if err := rt.buildPerf.ClearBuildLimitSetting(r.Context(), k); err != nil {
			rt.internalError(w, "api: reset build limits failed", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, rt.buildLimitsView(r.Context()))
}

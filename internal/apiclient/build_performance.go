package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// BuildTrend summarises an app's recent builds.
type BuildTrend struct {
	Builds        int     `json:"builds"`
	Succeeded     int     `json:"succeeded"`
	Failed        int     `json:"failed"`
	TimedOut      int     `json:"timed_out"`
	Canceled      int     `json:"canceled"`
	Skewed        int     `json:"skewed"`
	P50Seconds    float64 `json:"p50_seconds"`
	P95Seconds    float64 `json:"p95_seconds"`
	MinSeconds    float64 `json:"min_seconds"`
	MaxSeconds    float64 `json:"max_seconds"`
	CacheHitRatio float64 `json:"cache_hit_ratio"`
	CacheKnown    bool    `json:"cache_known"`
}

// BuildRun is one recorded build.
type BuildRun struct {
	StartedAt       time.Time `json:"started_at"`
	Tag             string    `json:"tag"`
	Status          string    `json:"status"`
	DurationSeconds float64   `json:"duration_seconds"`
	QueueWaitSecs   float64   `json:"queue_wait_seconds"`
	CachedSteps     int       `json:"cached_steps"`
	TotalSteps      int       `json:"total_steps"`
	Error           string    `json:"error,omitempty"`
}

// BuildStats is the response of GET /api/v1/apps/{name}/build-stats.
type BuildStats struct {
	App   string     `json:"app"`
	Limit int        `json:"limit"`
	Trend BuildTrend `json:"trend"`
	Runs  []BuildRun `json:"runs"`
}

// BuildLimitValues are build limits; 0 means unlimited.
type BuildLimitValues struct {
	MaxConcurrent  int `json:"max_concurrent"`
	TimeoutSeconds int `json:"timeout_seconds"`
	MemoryMB       int `json:"memory_mb"`
	CPUMillis      int `json:"cpu_millis"`
}

// BuildLimitOverrides are the limits set explicitly; nil means the default applies.
type BuildLimitOverrides struct {
	MaxConcurrent  *int `json:"max_concurrent,omitempty"`
	TimeoutSeconds *int `json:"timeout_seconds,omitempty"`
	MemoryMB       *int `json:"memory_mb,omitempty"`
	CPUMillis      *int `json:"cpu_millis,omitempty"`
}

// BuildLimits is the response of /api/v1/build-limits.
type BuildLimits struct {
	Effective BuildLimitValues    `json:"effective"`
	Default   BuildLimitValues    `json:"default"`
	Overrides BuildLimitOverrides `json:"overrides"`
}

// AppBuildLimits is the response of /api/v1/apps/{name}/build-limits.
type AppBuildLimits struct {
	Effective BuildLimitValues    `json:"effective"`
	Global    BuildLimitValues    `json:"global"`
	Overrides BuildLimitOverrides `json:"overrides"`
}

// BuildLimitsRequest sets build limit overrides; nil fields are left alone.
type BuildLimitsRequest = BuildLimitOverrides

// GetBuildStats calls GET /api/v1/apps/{name}/build-stats.
func (c *Client) GetBuildStats(ctx context.Context, app string, limit int) (BuildStats, error) {
	var out BuildStats
	path := "/api/v1/apps/" + url.PathEscape(app) + "/build-stats"
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetBuildLimits calls GET /api/v1/build-limits.
func (c *Client) GetBuildLimits(ctx context.Context) (BuildLimits, error) {
	var out BuildLimits
	err := c.do(ctx, http.MethodGet, "/api/v1/build-limits", nil, &out)
	return out, err
}

// SetBuildLimits calls PUT /api/v1/build-limits.
func (c *Client) SetBuildLimits(ctx context.Context, req BuildLimitsRequest) (BuildLimits, error) {
	var out BuildLimits
	err := c.do(ctx, http.MethodPut, "/api/v1/build-limits", req, &out)
	return out, err
}

// ResetBuildLimits calls DELETE /api/v1/build-limits.
func (c *Client) ResetBuildLimits(ctx context.Context) (BuildLimits, error) {
	var out BuildLimits
	err := c.do(ctx, http.MethodDelete, "/api/v1/build-limits", nil, &out)
	return out, err
}

// GetAppBuildLimits calls GET /api/v1/apps/{name}/build-limits.
func (c *Client) GetAppBuildLimits(ctx context.Context, app string) (AppBuildLimits, error) {
	var out AppBuildLimits
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+url.PathEscape(app)+"/build-limits", nil, &out)
	return out, err
}

// SetAppBuildLimits calls PUT /api/v1/apps/{name}/build-limits.
func (c *Client) SetAppBuildLimits(ctx context.Context, app string, req BuildLimitsRequest) (AppBuildLimits, error) {
	var out AppBuildLimits
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+url.PathEscape(app)+"/build-limits", req, &out)
	return out, err
}

// ResetAppBuildLimits calls DELETE /api/v1/apps/{name}/build-limits.
func (c *Client) ResetAppBuildLimits(ctx context.Context, app string) (AppBuildLimits, error) {
	var out AppBuildLimits
	err := c.do(ctx, http.MethodDelete, "/api/v1/apps/"+url.PathEscape(app)+"/build-limits", nil, &out)
	return out, err
}

// BuildSpeedFinding is one suggestion to make an app's builds faster.
type BuildSpeedFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	FixKind  string `json:"fix_kind"`
	TargetID string `json:"target_id,omitempty"`
}

// BuildSpeed is the response of GET /api/v1/apps/{name}/build-speed.
type BuildSpeed struct {
	Builds           int                 `json:"builds"`
	WarmP50Seconds   float64             `json:"warm_p50_seconds"`
	ColdP50Seconds   float64             `json:"cold_p50_seconds"`
	WarmKnown        bool                `json:"warm_known"`
	TimeSavedSeconds float64             `json:"time_saved_seconds"`
	Findings         []BuildSpeedFinding `json:"findings"`
}

// GetBuildSpeed calls GET /api/v1/apps/{name}/build-speed.
func (c *Client) GetBuildSpeed(ctx context.Context, app string) (BuildSpeed, error) {
	var out BuildSpeed
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+url.PathEscape(app)+"/build-speed", nil, &out)
	return out, err
}

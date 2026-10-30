package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// CPHealthFinding is one saturated measurement on the control plane.
type CPHealthFinding struct {
	ID        string  `json:"id"`
	Severity  string  `json:"severity"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Fix       string  `json:"fix,omitempty"`
}

// CPHealth is the /api/v1/system/control-plane-health resource.
type CPHealth struct {
	CheckedAt time.Time `json:"checked_at"`
	Status    string    `json:"status"`
	Runtime   struct {
		Goroutines int     `json:"goroutines"`
		HeapBytes  uint64  `json:"heap_bytes"`
		GCPauseMS  float64 `json:"gc_pause_ms"`
		CPUs       int     `json:"cpus"`
	} `json:"runtime"`
	DB struct {
		Queries         int64   `json:"queries"`
		SlowQueries     int64   `json:"slow_queries"`
		P50MS           float64 `json:"p50_ms"`
		P95MS           float64 `json:"p95_ms"`
		P99MS           float64 `json:"p99_ms"`
		WaitCount       int64   `json:"wait_count"`
		WaitMS          float64 `json:"wait_ms"`
		ReadPoolEnabled bool    `json:"read_pool_enabled"`
		ReadPoolSize    int     `json:"read_pool_size"`
		ReadInUse       int     `json:"read_in_use"`
		ReadWaitCount   int64   `json:"read_wait_count"`
		ReadWaitMS      float64 `json:"read_wait_ms"`
		WALBytes        int64   `json:"wal_bytes"`
		DBBytes         int64   `json:"db_bytes"`
	} `json:"db"`
	Reconcile struct {
		Passes      int64   `json:"passes"`
		Controllers int     `json:"controllers"`
		LastPassMS  float64 `json:"last_pass_ms"`
		P95MS       float64 `json:"p95_ms"`
		LagSeconds  float64 `json:"lag_seconds"`
		Guard       struct {
			PassesDeferred int64             `json:"passes_deferred"`
			RecentLimited  int               `json:"recent_limited"`
			OpenBreakers   int               `json:"open_breakers"`
			Breakers       []CPHealthBreaker `json:"breakers"`
		} `json:"guard"`
	} `json:"reconcile"`
	Streams struct {
		Active int64 `json:"active"`
		Cap    int64 `json:"cap"`
	} `json:"streams"`
	Footprint struct {
		RSSBytes       uint64  `json:"rss_bytes"`
		Threads        int     `json:"threads"`
		OpenFDs        int     `json:"open_fds"`
		CPUPercent     float64 `json:"cpu_percent"`
		TelemetryBytes int64   `json:"telemetry_bytes"`
		MemoryLimit    struct {
			LimitBytes int64  `json:"limit_bytes"`
			Source     string `json:"source"`
			Percent    int    `json:"percent"`
		} `json:"memory_limit"`
		Loops []CPHealthLoop `json:"loops"`
	} `json:"footprint"`
	Idle     footprint.Assessment `json:"idle_footprint"`
	Findings []CPHealthFinding    `json:"findings"`
}

// CPHealthBreaker is one controller's circuit breaker.
type CPHealthBreaker struct {
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Failures  int       `json:"failures"`
	OpenUntil time.Time `json:"open_until"`
	LastError string    `json:"last_error,omitempty"`
	Trips     int64     `json:"trips"`
	Panics    int64     `json:"panics"`
	Throttled int64     `json:"throttled"`
	Skipped   int64     `json:"skipped"`
}

// CPHealthLoop is one background loop with its last run and cost.
type CPHealthLoop struct {
	Name         string    `json:"name"`
	IntervalSecs float64   `json:"interval_seconds"`
	Runs         int64     `json:"runs"`
	Errors       int64     `json:"errors"`
	LastRunAt    time.Time `json:"last_run_at"`
	LastDuration float64   `json:"last_duration_ms"`
	CostPercent  float64   `json:"cost_percent"`
	Running      bool      `json:"running"`
	LastError    string    `json:"last_error,omitempty"`
}

// Probe issues GET path and discards the body, so the soak harness can
// generate read load without a typed method per endpoint.
func (c *Client) Probe(ctx context.Context, path string) error {
	var out json.RawMessage
	return c.do(ctx, http.MethodGet, path, nil, &out)
}

// GetCPHealth calls GET /api/v1/system/control-plane-health.
func (c *Client) GetCPHealth(ctx context.Context) (CPHealth, error) {
	var out CPHealth
	err := c.do(ctx, http.MethodGet, "/api/v1/system/control-plane-health", nil, &out)
	return out, err
}

// FixCPHealth calls POST /api/v1/system/control-plane-health/fix.
func (c *Client) FixCPHealth(ctx context.Context, fix string) (CPHealth, error) {
	var out CPHealth
	err := c.do(ctx, http.MethodPost, "/api/v1/system/control-plane-health/fix", map[string]string{"fix": fix}, &out)
	return out, err
}

// GetIdleFootprint calls GET /api/v1/system/footprint.
func (c *Client) GetIdleFootprint(ctx context.Context) (footprint.Assessment, error) {
	var out footprint.Assessment
	err := c.do(ctx, http.MethodGet, "/api/v1/system/footprint", nil, &out)
	return out, err
}

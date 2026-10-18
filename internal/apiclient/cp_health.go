package apiclient

import (
	"context"
	"net/http"
	"time"
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
		Queries     int64   `json:"queries"`
		SlowQueries int64   `json:"slow_queries"`
		P50MS       float64 `json:"p50_ms"`
		P95MS       float64 `json:"p95_ms"`
		P99MS       float64 `json:"p99_ms"`
		WaitCount   int64   `json:"wait_count"`
		WaitMS      float64 `json:"wait_ms"`
		WALBytes    int64   `json:"wal_bytes"`
		DBBytes     int64   `json:"db_bytes"`
	} `json:"db"`
	Reconcile struct {
		Passes      int64   `json:"passes"`
		Controllers int     `json:"controllers"`
		LastPassMS  float64 `json:"last_pass_ms"`
		P95MS       float64 `json:"p95_ms"`
		LagSeconds  float64 `json:"lag_seconds"`
	} `json:"reconcile"`
	Streams struct {
		Active int64 `json:"active"`
		Cap    int64 `json:"cap"`
	} `json:"streams"`
	Findings []CPHealthFinding `json:"findings"`
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

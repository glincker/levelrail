package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/benchreport"
)

// BenchmarkRunSummary mirrors one row of GET /api/v1/benchmarks.
type BenchmarkRunSummary struct {
	ID          string    `json:"id"`
	GeneratedAt time.Time `json:"generated_at"`
	Version     string    `json:"version"`
	Commit      string    `json:"commit"`
	Dirty       bool      `json:"dirty"`
	HostSummary string    `json:"host_summary"`
	HostOS      string    `json:"host_os"`
	Repeat      int       `json:"repeat"`
	Passed      int       `json:"passed"`
	Failed      int       `json:"failed"`
	Skipped     int       `json:"skipped"`
	RecordedBy  string    `json:"recorded_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// BenchmarkList mirrors GET /api/v1/benchmarks.
type BenchmarkList struct {
	Runs []BenchmarkRunSummary `json:"runs"`
	Keep int                   `json:"keep"`
}

// BenchmarkRun mirrors GET /api/v1/benchmarks/{id}.
type BenchmarkRun struct {
	Run    BenchmarkRunSummary `json:"run"`
	Report benchreport.Report  `json:"report"`
	Groups map[string]string   `json:"groups"`
}

// ListBenchmarks calls GET /api/v1/benchmarks.
func (c *Client) ListBenchmarks(ctx context.Context, limit int) (BenchmarkList, error) {
	path := "/api/v1/benchmarks"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out BenchmarkList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetBenchmark calls GET /api/v1/benchmarks/{id}.
func (c *Client) GetBenchmark(ctx context.Context, id string) (BenchmarkRun, error) {
	var out BenchmarkRun
	err := c.do(ctx, http.MethodGet, "/api/v1/benchmarks/"+PathEscape(id), nil, &out)
	return out, err
}

// CompareBenchmarks calls GET /api/v1/benchmarks/compare.
func (c *Client) CompareBenchmarks(ctx context.Context, base, head string) (benchreport.Comparison, error) {
	var out benchreport.Comparison
	q := url.Values{"base": {base}, "head": {head}}
	err := c.do(ctx, http.MethodGet, "/api/v1/benchmarks/compare?"+q.Encode(), nil, &out)
	return out, err
}

// UploadBenchmark calls POST /api/v1/benchmarks with a results file as written by bench chaos run --out.
func (c *Client) UploadBenchmark(ctx context.Context, file []byte) (BenchmarkRunSummary, error) {
	var out BenchmarkRunSummary
	err := c.do(ctx, http.MethodPost, "/api/v1/benchmarks", json.RawMessage(file), &out)
	return out, err
}

// DeleteBenchmark calls DELETE /api/v1/benchmarks/{id}.
func (c *Client) DeleteBenchmark(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/benchmarks/"+PathEscape(id), nil, nil)
}

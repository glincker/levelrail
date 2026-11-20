package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// BuilderResource mirrors one row of GET /api/v1/builders.
type BuilderResource struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	Paused        bool       `json:"paused"`
	Tier          string     `json:"tier"`
	OS            string     `json:"os,omitempty"`
	Arch          string     `json:"arch,omitempty"`
	AgentVersion  string     `json:"agent_version,omitempty"`
	CPUCores      int        `json:"cpu_cores,omitempty"`
	Pressure      string     `json:"pressure"`
	SlotsUsed     int        `json:"slots_used"`
	SlotsTotal    int        `json:"slots_total"`
	ActiveBuilds  int        `json:"active_builds"`
	Waiting       int        `json:"waiting"`
	AcceptsApps   bool       `json:"accepts_app_workloads"`
	Labels        []string   `json:"labels"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	LastBuildAt   *time.Time `json:"last_build_at,omitempty"`
	RecentBuilds  int        `json:"recent_builds"`
	RecentFailed  int        `json:"recent_failed"`
	AvgBuildMS    int64      `json:"avg_build_ms"`
	DiskFreeBytes int64      `json:"disk_free_bytes,omitempty"`
}

// BuilderBuildRow is one recent build on a builder.
type BuilderBuildRow struct {
	App        string    `json:"app"`
	Tag        string    `json:"tag"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	Result     string    `json:"result"`
}

// BuilderDetailResource mirrors GET /api/v1/builders/{id}.
type BuilderDetailResource struct {
	BuilderResource
	Builds []BuilderBuildRow `json:"builds"`
}

// BuildersResponse mirrors GET /api/v1/builders.
type BuildersResponse struct {
	Builders []BuilderResource `json:"builders"`
}

// BuilderTestResponse mirrors POST /api/v1/builders/{id}/test.
type BuilderTestResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	OK            bool   `json:"ok"`
	LatencyMS     int64  `json:"latency_ms"`
	Arch          string `json:"arch,omitempty"`
	OS            string `json:"os,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	BuildKit      string `json:"buildkit_version,omitempty"`
	DiskFreeBytes int64  `json:"disk_free_bytes,omitempty"`
	Error         string `json:"error,omitempty"`
}

func builderPath(id string) string { return "/api/v1/builders/" + url.PathEscape(id) }

// ListBuilders calls GET /api/v1/builders.
func (c *Client) ListBuilders(ctx context.Context) (BuildersResponse, error) {
	var out BuildersResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/builders", nil, &out)
	return out, err
}

// GetBuilder calls GET /api/v1/builders/{id}; id may be the builder name.
func (c *Client) GetBuilder(ctx context.Context, id string) (BuilderDetailResource, error) {
	var out BuilderDetailResource
	err := c.do(ctx, http.MethodGet, builderPath(id), nil, &out)
	return out, err
}

// PauseBuilder calls POST /api/v1/builders/{id}/pause.
func (c *Client) PauseBuilder(ctx context.Context, id string) (BuilderResource, error) {
	var out BuilderResource
	err := c.do(ctx, http.MethodPost, builderPath(id)+"/pause", nil, &out)
	return out, err
}

// ResumeBuilder calls POST /api/v1/builders/{id}/resume.
func (c *Client) ResumeBuilder(ctx context.Context, id string) (BuilderResource, error) {
	var out BuilderResource
	err := c.do(ctx, http.MethodPost, builderPath(id)+"/resume", nil, &out)
	return out, err
}

// TestBuilder calls POST /api/v1/builders/{id}/test.
func (c *Client) TestBuilder(ctx context.Context, id string) (BuilderTestResponse, error) {
	var out BuilderTestResponse
	err := c.do(ctx, http.MethodPost, builderPath(id)+"/test", nil, &out)
	return out, err
}

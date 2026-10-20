package apiclient

import (
	"context"
	"fmt"
	"net/http"
)

// PreviewLimitsResource mirrors internal/api's previewLimitsResource.
type PreviewLimitsResource struct {
	MaxPerApp int `json:"max_per_app"`
	MaxTotal  int `json:"max_total"`
	LiveTotal int `json:"live_total"`
}

// PreviewsOverview is GET /api/v1/previews's body.
type PreviewsOverview struct {
	Limits   PreviewLimitsResource        `json:"limits"`
	PerApp   map[string]int               `json:"live_per_app"`
	Previews []PreviewEnvironmentResource `json:"previews"`
}

// PreviewPolicyResource mirrors internal/api's previewPolicyResource.
type PreviewPolicyResource struct {
	OnLimit           string `json:"on_limit"`
	AllowForkPreviews bool   `json:"allow_fork_previews"`
	TTLHours          int    `json:"ttl_hours"`
	EffectiveTTLHours int    `json:"effective_ttl_hours"`
	MaxPerApp         int    `json:"max_per_app"`
	LiveCount         int    `json:"live_count"`
	MaxTotal          int    `json:"max_total"`
	LiveTotal         int    `json:"live_total"`

	MaxPreviews               int     `json:"max_previews"`
	MemoryLimit               string  `json:"memory_limit"`
	CPULimit                  float64 `json:"cpu_limit"`
	EffectiveMemory           string  `json:"effective_memory"`
	EffectiveCPU              float64 `json:"effective_cpu"`
	IdleSleepMinutes          int     `json:"idle_sleep_minutes"`
	EffectiveIdleSleepMinutes int     `json:"effective_idle_sleep_minutes"`
	DatabaseStrategy          string  `json:"database_strategy"`
	SeedDatabase              string  `json:"seed_database"`
	AllowForkSecrets          bool    `json:"allow_fork_secrets"`
	GateBasicAuth             bool    `json:"gate_basic_auth"`
	GateUsername              string  `json:"gate_username"`
	GatePasswordSet           bool    `json:"gate_password_set"`
	AllowIndexing             bool    `json:"allow_indexing"`
}

// SetPreviewPolicyRequest is PUT .../preview-policy's body; nil fields are left unchanged.
type SetPreviewPolicyRequest struct {
	OnLimit           *string `json:"on_limit,omitempty"`
	AllowForkPreviews *bool   `json:"allow_fork_previews,omitempty"`
	TTLHours          *int    `json:"ttl_hours,omitempty"`

	MaxPreviews      *int     `json:"max_previews,omitempty"`
	MemoryLimit      *string  `json:"memory_limit,omitempty"`
	CPULimit         *float64 `json:"cpu_limit,omitempty"`
	IdleSleepMinutes *int     `json:"idle_sleep_minutes,omitempty"`
	DatabaseStrategy *string  `json:"database_strategy,omitempty"`
	SeedDatabase     *string  `json:"seed_database,omitempty"`
	AllowForkSecrets *bool    `json:"allow_fork_secrets,omitempty"`
	GateBasicAuth    *bool    `json:"gate_basic_auth,omitempty"`
	GateUsername     *string  `json:"gate_username,omitempty"`
	GatePassword     *string  `json:"gate_password,omitempty"`
	AllowIndexing    *bool    `json:"allow_indexing,omitempty"`
}

type approvePreviewRequest struct {
	Confirm      bool `json:"confirm"`
	ShareSecrets bool `json:"share_secrets"`
}

type extendPreviewRequest struct {
	Hours int `json:"hours"`
}

// ApprovePreviewResult is POST .../approve's body.
type ApprovePreviewResult struct {
	Status string `json:"status"`
}

// ListAllPreviews calls GET /api/v1/previews: previews of every readable
// app plus platform-wide limit usage.
func (c *Client) ListAllPreviews(ctx context.Context) (PreviewsOverview, error) {
	var out PreviewsOverview
	err := c.do(ctx, http.MethodGet, "/api/v1/previews", nil, &out)
	return out, err
}

// GetPreviewPolicy calls GET /api/v1/apps/{name}/preview-policy.
func (c *Client) GetPreviewPolicy(ctx context.Context, appName string) (PreviewPolicyResource, error) {
	var out PreviewPolicyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/preview-policy", nil, &out)
	return out, err
}

// SetPreviewPolicy calls PUT /api/v1/apps/{name}/preview-policy.
func (c *Client) SetPreviewPolicy(ctx context.Context, appName string, req SetPreviewPolicyRequest) (PreviewPolicyResource, error) {
	var out PreviewPolicyResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/preview-policy", req, &out)
	return out, err
}

// ApprovePreviewEnvironment calls POST /api/v1/apps/{name}/previews/{number}/approve
// with the confirmation the server requires.
func (c *Client) ApprovePreviewEnvironment(ctx context.Context, appName string, prNumber int) (ApprovePreviewResult, error) {
	return c.ApprovePreviewEnvironmentShared(ctx, appName, prNumber, false)
}

// ApprovePreviewEnvironmentShared is ApprovePreviewEnvironment that can also
// give this one deploy the app's environment variables and secrets.
func (c *Client) ApprovePreviewEnvironmentShared(ctx context.Context, appName string, prNumber int, shareSecrets bool) (ApprovePreviewResult, error) {
	var out ApprovePreviewResult
	path := fmt.Sprintf("/api/v1/apps/%s/previews/%d/approve", PathEscape(appName), prNumber)
	err := c.do(ctx, http.MethodPost, path, approvePreviewRequest{Confirm: true, ShareSecrets: shareSecrets}, &out)
	return out, err
}

// ExtendPreviewEnvironment calls POST /api/v1/apps/{name}/previews/{number}/extend.
func (c *Client) ExtendPreviewEnvironment(ctx context.Context, appName string, prNumber, hours int) (PreviewEnvironmentResource, error) {
	var out PreviewEnvironmentResource
	path := fmt.Sprintf("/api/v1/apps/%s/previews/%d/extend", PathEscape(appName), prNumber)
	err := c.do(ctx, http.MethodPost, path, extendPreviewRequest{Hours: hours}, &out)
	return out, err
}

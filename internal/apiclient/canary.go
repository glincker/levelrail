package apiclient

import (
	"context"
	"net/http"
	"time"
)

// CanaryProgress mirrors internal/canary's Progress.
type CanaryProgress struct {
	Stage          int     `json:"stage"`
	StageCount     int     `json:"stage_count"`
	ElapsedSecs    int     `json:"elapsed_secs"`
	IntervalSecs   int     `json:"interval_secs"`
	CanaryRequests uint64  `json:"canary_requests"`
	CanaryErrRate  float64 `json:"canary_error_rate"`
	CanaryP95MS    float64 `json:"canary_p95_ms"`
	StableRequests uint64  `json:"stable_requests"`
	StableErrRate  float64 `json:"stable_error_rate"`
	StableP95MS    float64 `json:"stable_p95_ms"`
	Next           string  `json:"next"`
	Breaching      bool    `json:"breaching"`
}

// CanaryOutcome is how an app's last canary ended.
type CanaryOutcome struct {
	Outcome string    `json:"outcome"`
	Reason  string    `json:"reason,omitempty"`
	Image   string    `json:"image,omitempty"`
	At      time.Time `json:"at"`
}

// CanaryResource mirrors internal/api's canaryResource.
type CanaryResource struct {
	Active       bool            `json:"active"`
	Image        string          `json:"image,omitempty"`
	Weight       int             `json:"weight"`
	CreatedAt    *time.Time      `json:"created_at,omitempty"`
	Staged       bool            `json:"staged,omitempty"`
	Stages       []int           `json:"stages,omitempty"`
	Stage        int             `json:"stage,omitempty"`
	IntervalSecs int             `json:"interval_secs,omitempty"`
	MaxErrorRate float64         `json:"max_error_rate,omitempty"`
	MaxP95Ratio  float64         `json:"max_p95_ratio,omitempty"`
	StatusReason string          `json:"status_reason,omitempty"`
	Progress     *CanaryProgress `json:"progress,omitempty"`
	LastOutcome  *CanaryOutcome  `json:"last_outcome,omitempty"`
}

// CanaryStartRequest is the body of POST /api/v1/apps/{name}/canary. Weight
// starts a manual canary; Stages (or Staged) starts one the control plane walks
// up and rolls back on its own. Zero values use the server defaults.
type CanaryStartRequest struct {
	Image        string  `json:"image"`
	Weight       int     `json:"weight,omitempty"`
	Staged       bool    `json:"staged,omitempty"`
	Stages       []int   `json:"stages,omitempty"`
	Interval     string  `json:"interval,omitempty"`
	MaxErrorRate float64 `json:"max_error_rate,omitempty"`
	MaxP95Ratio  float64 `json:"max_p95_ratio,omitempty"`
}

func canaryPath(appName string) string { return "/api/v1/apps/" + PathEscape(appName) + "/canary" }

// GetCanary calls GET /api/v1/apps/{name}/canary.
func (c *Client) GetCanary(ctx context.Context, appName string) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodGet, canaryPath(appName), nil, &out)
	return out, err
}

// StartCanary calls POST /api/v1/apps/{name}/canary. A weight of 0 uses the server default.
func (c *Client) StartCanary(ctx context.Context, appName, image string, weight int) (CanaryResource, error) {
	return c.StartCanaryWith(ctx, appName, CanaryStartRequest{Image: image, Weight: weight})
}

// StartCanaryWith calls POST /api/v1/apps/{name}/canary with the full request.
func (c *Client) StartCanaryWith(ctx context.Context, appName string, req CanaryStartRequest) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodPost, canaryPath(appName), req, &out)
	return out, err
}

// SetCanaryWeight calls PUT /api/v1/apps/{name}/canary; 0 pauses the canary.
func (c *Client) SetCanaryWeight(ctx context.Context, appName string, weight int) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodPut, canaryPath(appName), map[string]int{"weight": weight}, &out)
	return out, err
}

// PromoteCanary calls POST /api/v1/apps/{name}/canary/promote.
func (c *Client) PromoteCanary(ctx context.Context, appName string) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodPost, canaryPath(appName)+"/promote", nil, &out)
	return out, err
}

// AbortCanary calls DELETE /api/v1/apps/{name}/canary.
func (c *Client) AbortCanary(ctx context.Context, appName string) (CanaryResource, error) {
	var out CanaryResource
	err := c.do(ctx, http.MethodDelete, canaryPath(appName), nil, &out)
	return out, err
}

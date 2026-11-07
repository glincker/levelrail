package apiclient

import (
	"context"
	"net/http"
	"time"
)

// RolloutHold is the post-flip hold countdown of the newest release.
type RolloutHold struct {
	State            string `json:"state"`
	Mode             string `json:"mode"`
	RemainingSeconds int    `json:"remaining_seconds"`
	WindowSeconds    int    `json:"window_seconds"`
	Reason           string `json:"reason,omitempty"`
	Message          string `json:"message,omitempty"`
}

// RolloutStateResource is GET /api/v1/apps/{name}/rollout/state.
type RolloutStateResource struct {
	App           string          `json:"app"`
	Strategy      string          `json:"strategy"`
	DeployID      string          `json:"deploy_id,omitempty"`
	Image         string          `json:"image,omitempty"`
	Stage         string          `json:"stage"`
	Traffic       string          `json:"traffic"`
	GateFailures  int             `json:"gate_failures"`
	AutoRollback  bool            `json:"auto_rollback"`
	FlippedAt     *time.Time      `json:"flipped_at,omitempty"`
	RolledBackAt  *time.Time      `json:"rolled_back_at,omitempty"`
	FailureReason string          `json:"failure_reason,omitempty"`
	Hold          RolloutHold     `json:"hold"`
	Canary        *CanaryResource `json:"canary,omitempty"`
}

// GetRolloutState calls GET /api/v1/apps/{name}/rollout/state.
func (c *Client) GetRolloutState(ctx context.Context, appName string) (RolloutStateResource, error) {
	var out RolloutStateResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/rollout/state", nil, &out)
	return out, err
}

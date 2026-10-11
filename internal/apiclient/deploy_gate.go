package apiclient

import (
	"context"
	"net/http"
	"time"
)

// GateEventResource is one step of a deploy's gate timeline.
type GateEventResource struct {
	Phase     string    `json:"phase"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Message   string    `json:"message"`
	Container string    `json:"container,omitempty"`
	At        time.Time `json:"at"`
}

// DeploySafetySettingsResource is a fully resolved set of gate and watch settings.
type DeploySafetySettingsResource struct {
	SuccessThreshold int     `json:"success_threshold"`
	StableWindow     string  `json:"stable_window"`
	DrainTimeout     string  `json:"drain_timeout"`
	WatchWindow      string  `json:"watch_window"`
	WatchMaxRestarts int     `json:"watch_max_restarts"`
	WatchErrorRate   float64 `json:"watch_error_rate"`
	WatchMinRequests int     `json:"watch_min_requests"`
	AutoRollback     bool    `json:"auto_rollback"`
	RollbackPins     int     `json:"rollback_pins"`
}

// DeploySafetyOverrides is one app's overrides; a nil field inherits the default.
type DeploySafetyOverrides struct {
	SuccessThreshold *int     `json:"success_threshold"`
	StableWindow     *string  `json:"stable_window"`
	DrainTimeout     *string  `json:"drain_timeout"`
	WatchWindow      *string  `json:"watch_window"`
	WatchMaxRestarts *int     `json:"watch_max_restarts"`
	WatchErrorRate   *float64 `json:"watch_error_rate"`
	WatchMinRequests *int     `json:"watch_min_requests"`
	AutoRollback     *bool    `json:"auto_rollback"`
}

// DeploySafetyResource is GET and PUT /api/v1/apps/{name}/deploy-safety.
type DeploySafetyResource struct {
	Overrides DeploySafetyOverrides        `json:"overrides"`
	Defaults  DeploySafetySettingsResource `json:"defaults"`
	Effective DeploySafetySettingsResource `json:"effective"`
}

// RollbackTargetResource is a pinned release image cleanup never removes.
type RollbackTargetResource struct {
	DeployID string    `json:"deploy_id"`
	Image    string    `json:"image"`
	ImageID  string    `json:"image_id,omitempty"`
	PinRef   string    `json:"pin_ref"`
	PinnedAt time.Time `json:"pinned_at"`
	Live     bool      `json:"live"`
}

// DeployGateResource is GET /api/v1/apps/{name}/deploys/{deployId}/gate.
type DeployGateResource struct {
	DeployID    string                       `json:"deploy_id"`
	Image       string                       `json:"image"`
	Status      string                       `json:"status"`
	Reason      string                       `json:"reason,omitempty"`
	Error       string                       `json:"error,omitempty"`
	Stage       string                       `json:"stage"`
	Terminal    bool                         `json:"terminal"`
	Failed      bool                         `json:"failed"`
	Superseded  bool                         `json:"superseded"`
	Events      []GateEventResource          `json:"events"`
	Settings    DeploySafetySettingsResource `json:"settings"`
	RollbackPin *RollbackTargetResource      `json:"rollback_pin"`
}

// GetDeployGate calls GET /api/v1/apps/{name}/deploys/{deployId}/gate.
func (c *Client) GetDeployGate(ctx context.Context, appName, deployID string) (DeployGateResource, error) {
	var out DeployGateResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/deploys/"+PathEscape(deployID)+"/gate", nil, &out)
	return out, err
}

// GetDeploySafety calls GET /api/v1/apps/{name}/deploy-safety.
func (c *Client) GetDeploySafety(ctx context.Context, appName string) (DeploySafetyResource, error) {
	var out DeploySafetyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/deploy-safety", nil, &out)
	return out, err
}

// SetDeploySafety calls PUT /api/v1/apps/{name}/deploy-safety, replacing every override.
func (c *Client) SetDeploySafety(ctx context.Context, appName string, overrides DeploySafetyOverrides) (DeploySafetyResource, error) {
	var out DeploySafetyResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/deploy-safety", overrides, &out)
	return out, err
}

// ListRollbackTargets calls GET /api/v1/apps/{name}/rollback-targets.
func (c *Client) ListRollbackTargets(ctx context.Context, appName string) ([]RollbackTargetResource, error) {
	var out struct {
		Targets []RollbackTargetResource `json:"targets"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/rollback-targets", nil, &out)
	return out.Targets, err
}

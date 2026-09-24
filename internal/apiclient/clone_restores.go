package apiclient

import (
	"context"
	"net/http"
)

// DeployStepEvent mirrors internal/api's sseStepEvent.
type DeployStepEvent struct {
	Step      string `json:"step"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ListCloneRestores calls GET /api/v1/databases/{name}/clone-restores.
func (c *Client) ListCloneRestores(ctx context.Context, name string) ([]CloneRestoreResource, error) {
	var out []CloneRestoreResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/clone-restores", nil, &out)
	return out, err
}

// ListVolumeCloneRestores calls GET /api/v1/apps/{name}/volumes/{volume}/clone-restores.
func (c *Client) ListVolumeCloneRestores(ctx context.Context, name, volume string) ([]VolumeCloneRestoreResource, error) {
	var out []VolumeCloneRestoreResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/volumes/"+PathEscape(volume)+"/clone-restores", nil, &out)
	return out, err
}

// GetDatabaseStatus calls GET /api/v1/databases/{name}/status.
func (c *Client) GetDatabaseStatus(ctx context.Context, name string) ([]ConditionResource, error) {
	var out []ConditionResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/status", nil, &out)
	return out, err
}

// StreamDeploySteps calls GET /api/v1/apps/{name}/deploys/{deployId}/steps
// and calls onStep for each step event until onStep returns an error (the
// server holds the connection open after the last step) or ctx is canceled.
func (c *Client) StreamDeploySteps(ctx context.Context, name, deployID string, onStep func(DeployStepEvent) error) error {
	path := "/api/v1/apps/" + PathEscape(name) + "/deploys/" + PathEscape(deployID) + "/steps"
	return streamSSE(ctx, c, path, onStep)
}

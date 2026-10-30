package apiclient

import (
	"context"
	"net/http"
)

// PredeploySnapshotSettings is an app's pre-deploy database snapshot policy.
type PredeploySnapshotSettings struct {
	Mode     string `json:"mode"`
	TargetID string `json:"target_id"`
}

// PredeploySnapshot is one database snapshot taken before a deploy.
type PredeploySnapshot struct {
	ID               string `json:"id"`
	DeployID         string `json:"deploy_id,omitempty"`
	PriorDeployID    string `json:"prior_deploy_id,omitempty"`
	DatabaseName     string `json:"database_name"`
	Engine           string `json:"engine,omitempty"`
	BackupID         string `json:"backup_id,omitempty"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	SizeBytes        int64  `json:"size_bytes"`
	CreatedAt        string `json:"created_at"`
	RestoredDatabase string `json:"restored_database,omitempty"`
	RestoreStatus    string `json:"restore_status,omitempty"`
	RestoreError     string `json:"restore_error,omitempty"`
}

// RollbackWithDataResult is the accepted rollback-with-data operation.
type RollbackWithDataResult struct {
	DeployID   string              `json:"deploy_id"`
	RollbackTo string              `json:"rollback_to"`
	Snapshots  []PredeploySnapshot `json:"snapshots"`
}

// GetPredeploySnapshotSettings calls GET /api/v1/apps/{name}/predeploy-snapshot.
func (c *Client) GetPredeploySnapshotSettings(ctx context.Context, app string) (PredeploySnapshotSettings, error) {
	var out PredeploySnapshotSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/predeploy-snapshot", nil, &out)
	return out, err
}

// SetPredeploySnapshotSettings calls PUT /api/v1/apps/{name}/predeploy-snapshot.
func (c *Client) SetPredeploySnapshotSettings(ctx context.Context, app string, s PredeploySnapshotSettings) (PredeploySnapshotSettings, error) {
	var out PredeploySnapshotSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(app)+"/predeploy-snapshot", s, &out)
	return out, err
}

// ListPredeploySnapshots calls GET /api/v1/apps/{name}/predeploy-snapshots.
func (c *Client) ListPredeploySnapshots(ctx context.Context, app string) ([]PredeploySnapshot, error) {
	var out []PredeploySnapshot
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/predeploy-snapshots", nil, &out)
	return out, err
}

// RollbackWithData calls POST /api/v1/apps/{name}/deploys/{deployId}/rollback-with-data.
func (c *Client) RollbackWithData(ctx context.Context, app, deployID string) (RollbackWithDataResult, error) {
	var out RollbackWithDataResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/deploys/"+PathEscape(deployID)+"/rollback-with-data", nil, &out)
	return out, err
}

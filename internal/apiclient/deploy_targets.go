package apiclient

import (
	"context"
)

// DeployTargetNode is one node a workload can be placed on.
type DeployTargetNode struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Health  string            `json:"health"`
	Region  string            `json:"region,omitempty"`
	Tags    map[string]string `json:"tags,omitempty"`
	IsLocal bool              `json:"is_local,omitempty"`
}

// DeployTargetGroup is a cloud account, the local server or unmanaged nodes.
type DeployTargetGroup struct {
	Kind      string             `json:"kind"`
	AccountID string             `json:"account_id,omitempty"`
	Provider  string             `json:"provider,omitempty"`
	Label     string             `json:"label"`
	Nodes     []DeployTargetNode `json:"nodes"`
}

// ListDeployTargets calls GET /api/v1/deploy-targets.
func (c *Client) ListDeployTargets(ctx context.Context) ([]DeployTargetGroup, error) {
	return getJSON[[]DeployTargetGroup](ctx, c, "/api/v1/deploy-targets")
}

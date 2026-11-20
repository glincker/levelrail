package apiclient

import (
	"context"
	"net/http"
)

// NodeBuildTrustResource mirrors PUT /api/v1/nodes/{id}/build-trust.
type NodeBuildTrustResource struct {
	ID         string `json:"id"`
	BuildTrust string `json:"build_trust"`
}

// EnvironmentBuildersResource mirrors GET and PUT /api/v1/environments/{id}/builders.
type EnvironmentBuildersResource struct {
	EnvironmentID   string   `json:"environment_id"`
	Kind            string   `json:"kind"`
	AllowedBuilders []string `json:"allowed_builders"`
	PersonalAllowed bool     `json:"personal_allowed"`
}

// SetNodeBuildTrust calls PUT /api/v1/nodes/{id}/build-trust with tier server, managed or personal.
func (c *Client) SetNodeBuildTrust(ctx context.Context, id, tier string) (NodeBuildTrustResource, error) {
	var out NodeBuildTrustResource
	err := c.do(ctx, http.MethodPut, nodePath(id)+"/build-trust", map[string]string{"build_trust": tier}, &out)
	return out, err
}

// GetEnvironmentBuilders calls GET /api/v1/environments/{id}/builders.
func (c *Client) GetEnvironmentBuilders(ctx context.Context, id string) (EnvironmentBuildersResource, error) {
	var out EnvironmentBuildersResource
	err := c.do(ctx, http.MethodGet, environmentPath(id)+"/builders", nil, &out)
	return out, err
}

// SetEnvironmentBuilders calls PUT /api/v1/environments/{id}/builders; an empty list restores the default policy.
func (c *Client) SetEnvironmentBuilders(ctx context.Context, id string, allowed []string) (EnvironmentBuildersResource, error) {
	var out EnvironmentBuildersResource
	if allowed == nil {
		allowed = []string{}
	}
	err := c.do(ctx, http.MethodPut, environmentPath(id)+"/builders", map[string][]string{"allowed_builders": allowed}, &out)
	return out, err
}

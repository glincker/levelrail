package apiclient

import (
	"context"
	"net/http"
)

// SetComposeSourceRequest is PUT /api/v1/apps/{name}/compose-source's body.
type SetComposeSourceRequest struct {
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch,omitempty"`
	ComposePath string `json:"compose_path,omitempty"`
	TriggerMode string `json:"trigger_mode,omitempty"`
}

// GetComposeSource calls GET /api/v1/apps/{name}/compose-source.
func (c *Client) GetComposeSource(ctx context.Context, name string) (GitSourceResource, error) {
	var out GitSourceResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/compose-source", nil, &out)
	return out, err
}

// SetComposeSource calls PUT /api/v1/apps/{name}/compose-source; WebhookSecret is set only on create.
func (c *Client) SetComposeSource(ctx context.Context, name string, req SetComposeSourceRequest) (GitSourceResource, error) {
	var out GitSourceResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/compose-source", req, &out)
	return out, err
}

// DeployComposeSource calls POST /api/v1/apps/{name}/compose-source/deploy.
func (c *Client) DeployComposeSource(ctx context.Context, name string) (ComposeDeployResult, error) {
	var out ComposeDeployResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/compose-source/deploy", nil, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
)

// AppGitHubActionsResource mirrors internal/api's appGitHubActionsResource
// (GET/PUT /api/v1/apps/{name}/github-actions).
type AppGitHubActionsResource struct {
	Environment       string `json:"environment"`
	ReportDeployments bool   `json:"report_deployments"`
	ReportChecks      bool   `json:"report_checks"`
}

// GetAppGitHubActions calls GET /api/v1/apps/{name}/github-actions.
func (c *Client) GetAppGitHubActions(ctx context.Context, name string) (AppGitHubActionsResource, error) {
	var out AppGitHubActionsResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/github-actions", nil, &out)
	return out, err
}

// SetAppGitHubActions calls PUT /api/v1/apps/{name}/github-actions.
func (c *Client) SetAppGitHubActions(ctx context.Context, name string, req AppGitHubActionsResource) (AppGitHubActionsResource, error) {
	var out AppGitHubActionsResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/github-actions", req, &out)
	return out, err
}

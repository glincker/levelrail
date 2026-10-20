package apiclient

import (
	"context"
	"net/http"
)

// BranchDeploySettings mirrors GET /api/v1/apps/{name}/branch-deploys's settings object.
type BranchDeploySettings struct {
	Mode             string   `json:"mode"`
	Patterns         []string `json:"patterns"`
	ProductionBranch string   `json:"production_branch"`
	IdleTTLHours     int      `json:"idle_ttl_hours"`
}

// BranchDeploy is one live branch deployment.
type BranchDeploy struct {
	Branch       string `json:"branch"`
	Slug         string `json:"slug"`
	DeployAppID  string `json:"deploy_app_id"`
	URL          string `json:"url,omitempty"`
	Domain       string `json:"domain,omitempty"`
	HeadSHA      string `json:"head_sha"`
	HeadMessage  string `json:"head_message,omitempty"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// BranchDeploys is the settings plus live branch deploys of one app.
type BranchDeploys struct {
	Settings BranchDeploySettings `json:"settings"`
	Deploys  []BranchDeploy       `json:"deploys"`
}

type setBranchDeploySettingsRequest struct {
	Mode     string   `json:"mode"`
	Patterns []string `json:"patterns"`
}

// ListBranchDeploys calls GET /api/v1/apps/{name}/branch-deploys.
func (c *Client) ListBranchDeploys(ctx context.Context, name string) (BranchDeploys, error) {
	var out BranchDeploys
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/branch-deploys", nil, &out)
	return out, err
}

// SetBranchDeploySettings calls PUT /api/v1/apps/{name}/branch-deploys.
func (c *Client) SetBranchDeploySettings(ctx context.Context, name, mode string, patterns []string) (BranchDeploys, error) {
	var out BranchDeploys
	if patterns == nil {
		patterns = []string{}
	}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/branch-deploys", setBranchDeploySettingsRequest{Mode: mode, Patterns: patterns}, &out)
	return out, err
}

// DeleteBranchDeploy calls DELETE /api/v1/apps/{name}/branch-deploys/{slug}.
func (c *Client) DeleteBranchDeploy(ctx context.Context, name, slug string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/branch-deploys/"+PathEscape(slug), nil, nil)
}

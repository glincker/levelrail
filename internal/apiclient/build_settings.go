package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppBuildSettings mirrors GET /api/v1/apps/{name}/build-settings.
type AppBuildSettings struct {
	Builder        string   `json:"builder"`
	DockerfilePath string   `json:"dockerfile_path"`
	Target         string   `json:"target"`
	NoCache        bool     `json:"no_cache"`
	Provider       string   `json:"provider"`
	HasGitSource   bool     `json:"has_git_source"`
	Providers      []string `json:"providers"`
	MaxValueBytes  int      `json:"max_value_bytes"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
}

// SetAppBuildSettingsRequest is the PUT body; nil fields keep their value.
type SetAppBuildSettingsRequest struct {
	Builder        *string `json:"builder,omitempty"`
	DockerfilePath *string `json:"dockerfile_path,omitempty"`
	Target         *string `json:"target,omitempty"`
	NoCache        *bool   `json:"no_cache,omitempty"`
	Provider       *string `json:"provider,omitempty"`
}

// GetAppBuildSettings calls GET /api/v1/apps/{name}/build-settings.
func (c *Client) GetAppBuildSettings(ctx context.Context, app string) (AppBuildSettings, error) {
	var out AppBuildSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+url.PathEscape(app)+"/build-settings", nil, &out)
	return out, err
}

// SetAppBuildSettings calls PUT /api/v1/apps/{name}/build-settings.
func (c *Client) SetAppBuildSettings(ctx context.Context, app string, req SetAppBuildSettingsRequest) (AppBuildSettings, error) {
	var out AppBuildSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+url.PathEscape(app)+"/build-settings", req, &out)
	return out, err
}

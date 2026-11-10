package apiclient

import (
	"context"
	"net/http"
	"time"
)

// BuildHookResource mirrors internal/api's buildHookResource; Path and Token are set only on create and rotate.
type BuildHookResource struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Branch          string     `json:"branch,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	LastResult      string     `json:"last_result,omitempty"`
	TriggerCount    int        `json:"trigger_count"`
	AllowedCIDRs    []string   `json:"allowed_cidrs"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	LastRefusal     string     `json:"last_refusal,omitempty"`
	Path            string     `json:"path,omitempty"`
	Token           string     `json:"token,omitempty"`
}

func buildHooksPath(appName string) string {
	return "/api/v1/apps/" + PathEscape(appName) + "/build-hooks"
}

// ListBuildHooks calls GET /api/v1/apps/{name}/build-hooks.
func (c *Client) ListBuildHooks(ctx context.Context, appName string) ([]BuildHookResource, error) {
	var out []BuildHookResource
	err := c.do(ctx, http.MethodGet, buildHooksPath(appName), nil, &out)
	return out, err
}

// CreateBuildHook calls POST /api/v1/apps/{name}/build-hooks; the secret URL comes back once.
// allowedCIDRs, when non-empty, limits which source addresses may fire the hook.
func (c *Client) CreateBuildHook(ctx context.Context, appName, name, branch string, allowedCIDRs []string) (BuildHookResource, error) {
	var out BuildHookResource
	body := map[string]any{"name": name, "branch": branch}
	if len(allowedCIDRs) > 0 {
		body["allowed_cidrs"] = allowedCIDRs
	}
	err := c.do(ctx, http.MethodPost, buildHooksPath(appName), body, &out)
	return out, err
}

// SetBuildHookAllowedCIDRs calls PATCH /api/v1/apps/{name}/build-hooks/{id}; an empty list sends the explicit allow_any the server requires to clear.
func (c *Client) SetBuildHookAllowedCIDRs(ctx context.Context, appName, id string, cidrs []string) (BuildHookResource, error) {
	var out BuildHookResource
	body := map[string]any{"allowed_cidrs": cidrs}
	if len(cidrs) == 0 {
		body = map[string]any{"allow_any": true}
	}
	err := c.do(ctx, http.MethodPatch, buildHooksPath(appName)+"/"+PathEscape(id), body, &out)
	return out, err
}

// RotateBuildHook calls POST /api/v1/apps/{name}/build-hooks/{id}/rotate.
func (c *Client) RotateBuildHook(ctx context.Context, appName, id string) (BuildHookResource, error) {
	var out BuildHookResource
	err := c.do(ctx, http.MethodPost, buildHooksPath(appName)+"/"+PathEscape(id)+"/rotate", nil, &out)
	return out, err
}

// RevokeBuildHook calls DELETE /api/v1/apps/{name}/build-hooks/{id}.
func (c *Client) RevokeBuildHook(ctx context.Context, appName, id string) error {
	return c.do(ctx, http.MethodDelete, buildHooksPath(appName)+"/"+PathEscape(id), nil, nil)
}

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
func (c *Client) CreateBuildHook(ctx context.Context, appName, name, branch string) (BuildHookResource, error) {
	var out BuildHookResource
	err := c.do(ctx, http.MethodPost, buildHooksPath(appName), map[string]string{"name": name, "branch": branch}, &out)
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

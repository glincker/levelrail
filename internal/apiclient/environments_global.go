package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// GlobalEnvironmentResource mirrors internal/api's environmentListResource,
// one row of GET /api/v1/environments.
type GlobalEnvironmentResource struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	Name          string `json:"name"`
	Protected     bool   `json:"protected"`
	CreatedAt     string `json:"created_at"`
	Kind          string `json:"kind"`
	Scope         string `json:"scope"`
	SortOrder     int    `json:"sort_order"`
	AppCount      int    `json:"app_count"`
	DatabaseCount int    `json:"database_count"`
}

// CreateGlobalEnvironmentRequest mirrors internal/api's createGlobalEnvironmentRequest.
type CreateGlobalEnvironmentRequest struct {
	Name      string `json:"name"`
	Kind      string `json:"kind,omitempty"`
	Protected bool   `json:"protected,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

// PatchEnvironmentRequest mirrors internal/api's updateEnvironmentRequest;
// nil fields are left unchanged.
type PatchEnvironmentRequest struct {
	Name      *string `json:"name,omitempty"`
	Kind      *string `json:"kind,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
	Protected *bool   `json:"protected,omitempty"`
}

// MoveEnvironmentRequest is the body of PUT .../{name}/environment.
type MoveEnvironmentRequest struct {
	EnvironmentID string `json:"environment_id"`
	Confirm       bool   `json:"confirm,omitempty"`
}

// MoveEnvironmentResult is the 200 resource or the 202 pending approval a
// move into or out of a protected environment returns.
type MoveEnvironmentResult struct {
	Name            string                  `json:"name"`
	EnvironmentID   string                  `json:"environment_id"`
	PendingApproval *DeployApprovalResource `json:"pending_approval,omitempty"`
}

// ListAllEnvironments calls GET /api/v1/environments.
func (c *Client) ListAllEnvironments(ctx context.Context) ([]GlobalEnvironmentResource, error) {
	var out []GlobalEnvironmentResource
	err := c.do(ctx, http.MethodGet, "/api/v1/environments", nil, &out)
	return out, err
}

// CreateGlobalEnvironment calls POST /api/v1/environments.
func (c *Client) CreateGlobalEnvironment(ctx context.Context, req CreateGlobalEnvironmentRequest) (EnvironmentResource, error) {
	var out EnvironmentResource
	err := c.do(ctx, http.MethodPost, "/api/v1/environments", req, &out)
	return out, err
}

// PatchEnvironment calls PATCH /api/v1/environments/{id}.
func (c *Client) PatchEnvironment(ctx context.Context, id string, req PatchEnvironmentRequest) (EnvironmentResource, error) {
	var out EnvironmentResource
	err := c.do(ctx, http.MethodPatch, environmentPath(id), req, &out)
	return out, err
}

// DeleteEnvironmentMoveTo calls DELETE /api/v1/environments/{id}, retagging
// every member to moveTo first when moveTo is not empty.
func (c *Client) DeleteEnvironmentMoveTo(ctx context.Context, id, moveTo string) error {
	path := environmentPath(id)
	if moveTo != "" {
		path += "?move_to=" + url.QueryEscape(moveTo)
	}
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// MoveAppEnvironment calls PUT /api/v1/apps/{name}/environment.
func (c *Client) MoveAppEnvironment(ctx context.Context, name string, req MoveEnvironmentRequest) (MoveEnvironmentResult, error) {
	var out MoveEnvironmentResult
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/environment", req, &out)
	return out, err
}

// MoveDatabaseEnvironment calls PUT /api/v1/databases/{name}/environment.
func (c *Client) MoveDatabaseEnvironment(ctx context.Context, name string, req MoveEnvironmentRequest) (MoveEnvironmentResult, error) {
	var out MoveEnvironmentResult
	err := c.do(ctx, http.MethodPut, "/api/v1/databases/"+PathEscape(name)+"/environment", req, &out)
	return out, err
}

// ListAppsInEnvironment calls GET /api/v1/apps?environment=<id or name>.
func (c *Client) ListAppsInEnvironment(ctx context.Context, environment string) ([]AppResource, error) {
	var out []AppResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps?environment="+url.QueryEscape(environment), nil, &out)
	return out, err
}

// ListDatabasesInEnvironment calls GET /api/v1/databases?environment=<id or name>.
func (c *Client) ListDatabasesInEnvironment(ctx context.Context, environment string) ([]DatabaseResource, error) {
	var out []DatabaseResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases?environment="+url.QueryEscape(environment), nil, &out)
	return out, err
}

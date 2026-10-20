package apiclient

import (
	"context"
	"net/http"
)

// RoleRequest is the body of POST /api/v1/roles and PUT /api/v1/roles/{id}.
type RoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Abilities   []string `json:"abilities"`
	Visibility  string   `json:"visibility,omitempty"`
}

// EnvironmentGrants is the body of the user environment-grants routes.
type EnvironmentGrants struct {
	EnvironmentIDs []string `json:"environment_ids"`
}

// CreateRole calls POST /api/v1/roles.
func (c *Client) CreateRole(ctx context.Context, req RoleRequest) (RoleResource, error) {
	var out RoleResource
	err := c.do(ctx, http.MethodPost, "/api/v1/roles", req, &out)
	return out, err
}

// UpdateRole calls PUT /api/v1/roles/{id}.
func (c *Client) UpdateRole(ctx context.Context, id string, req RoleRequest) (RoleResource, error) {
	var out RoleResource
	err := c.do(ctx, http.MethodPut, "/api/v1/roles/"+PathEscape(id), req, &out)
	return out, err
}

// DeleteRole calls DELETE /api/v1/roles/{id}.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/roles/"+PathEscape(id), nil, nil)
}

// SetUserRole calls PUT /api/v1/users/{id}/role.
func (c *Client) SetUserRole(ctx context.Context, userID, roleID string) (UserResource, error) {
	var out UserResource
	err := c.do(ctx, http.MethodPut, "/api/v1/users/"+PathEscape(userID)+"/role", map[string]string{"role_id": roleID}, &out)
	return out, err
}

// GetUserEnvironmentGrants calls GET /api/v1/users/{id}/environment-grants.
func (c *Client) GetUserEnvironmentGrants(ctx context.Context, userID string) (EnvironmentGrants, error) {
	var out EnvironmentGrants
	err := c.do(ctx, http.MethodGet, "/api/v1/users/"+PathEscape(userID)+"/environment-grants", nil, &out)
	return out, err
}

// SetUserEnvironmentGrants calls PUT /api/v1/users/{id}/environment-grants.
func (c *Client) SetUserEnvironmentGrants(ctx context.Context, userID string, environmentIDs []string) (EnvironmentGrants, error) {
	if environmentIDs == nil {
		environmentIDs = []string{}
	}
	var out EnvironmentGrants
	err := c.do(ctx, http.MethodPut, "/api/v1/users/"+PathEscape(userID)+"/environment-grants", EnvironmentGrants{EnvironmentIDs: environmentIDs}, &out)
	return out, err
}

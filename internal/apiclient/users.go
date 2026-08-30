package apiclient

import (
	"context"
	"net/http"
	"time"
)

// UserResource mirrors internal/api's userResource (internal/api/users.go).
type UserResource struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	HasPassword bool       `json:"has_password"`
	Providers   []string   `json:"providers"`
	Abilities   []string   `json:"abilities"`
	Role        string     `json:"role,omitempty"`
	IsFirstUser bool       `json:"is_first_user"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// CreateUserRequest mirrors internal/api's createUserRequest. Role, when
// set, takes precedence server-side over Abilities (roles.go's
// resolveAbilities): set one or the other, not both.
type CreateUserRequest struct {
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name,omitempty"`
	Password    string   `json:"password"`
	Abilities   []string `json:"abilities,omitempty"`
	Role        string   `json:"role,omitempty"`
}

// UpdateUserAbilitiesRequest mirrors internal/api's
// updateUserAbilitiesRequest, same Role/Abilities precedence as
// CreateUserRequest.
type UpdateUserAbilitiesRequest struct {
	Abilities []string `json:"abilities,omitempty"`
	Role      string   `json:"role,omitempty"`
}

// RoleResource mirrors internal/api's Role (internal/api/roles.go).
type RoleResource struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Abilities   []string `json:"abilities"`
}

// CreateUser calls POST /api/v1/auth/users.
func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (UserResource, error) {
	var out UserResource
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/users", req, &out)
	return out, err
}

// ListUsers calls GET /api/v1/users.
func (c *Client) ListUsers(ctx context.Context) ([]UserResource, error) {
	var out []UserResource
	err := c.do(ctx, http.MethodGet, "/api/v1/users", nil, &out)
	return out, err
}

// UpdateUserAbilities calls PUT /api/v1/users/{id}/abilities.
func (c *Client) UpdateUserAbilities(ctx context.Context, id string, req UpdateUserAbilitiesRequest) (UserResource, error) {
	var out UserResource
	err := c.do(ctx, http.MethodPut, "/api/v1/users/"+PathEscape(id)+"/abilities", req, &out)
	return out, err
}

// DeleteUser calls DELETE /api/v1/users/{id}.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/users/"+PathEscape(id), nil, nil)
}

// ListRoles calls GET /api/v1/roles.
func (c *Client) ListRoles(ctx context.Context) ([]RoleResource, error) {
	var out []RoleResource
	err := c.do(ctx, http.MethodGet, "/api/v1/roles", nil, &out)
	return out, err
}

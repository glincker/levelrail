package apiclient

import (
	"context"
	"net/http"
)

// SecretRotationPolicyResource mirrors the control plane's rotation policy
// wire shape. It never carries a secret value.
type SecretRotationPolicyResource struct {
	App               string `json:"app,omitempty"`
	Key               string `json:"key"`
	IntervalDays      int    `json:"interval_days"`
	GraceHours        int    `json:"grace_hours"`
	Owner             string `json:"owner"`
	AutoRotate        bool   `json:"auto_rotate"`
	LastRotatedAt     string `json:"last_rotated_at,omitempty"`
	NextDueAt         string `json:"next_due_at,omitempty"`
	Status            string `json:"status"`
	State             string `json:"state"`
	LastError         string `json:"last_error,omitempty"`
	PreviousExpiresAt string `json:"previous_expires_at,omitempty"`
}

// SetSecretRotationPolicyRequest is the PUT body for a rotation policy.
type SetSecretRotationPolicyRequest struct {
	IntervalDays int    `json:"interval_days"`
	GraceHours   int    `json:"grace_hours"`
	Owner        string `json:"owner"`
	AutoRotate   bool   `json:"auto_rotate"`
}

// ListSecretRotation calls GET /api/v1/apps/{name}/secret-rotation.
func (c *Client) ListSecretRotation(ctx context.Context, name string) ([]SecretRotationPolicyResource, error) {
	var out []SecretRotationPolicyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/secret-rotation", nil, &out)
	return out, err
}

// ListAllSecretRotation calls GET /api/v1/secret-rotation (root only).
func (c *Client) ListAllSecretRotation(ctx context.Context) ([]SecretRotationPolicyResource, error) {
	var out []SecretRotationPolicyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/secret-rotation", nil, &out)
	return out, err
}

// SetSecretRotation calls PUT /api/v1/apps/{name}/secrets/{key}/rotation.
func (c *Client) SetSecretRotation(ctx context.Context, name, key string, req SetSecretRotationPolicyRequest) (SecretRotationPolicyResource, error) {
	var out SecretRotationPolicyResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/secrets/"+PathEscape(key)+"/rotation", req, &out)
	return out, err
}

// DeleteSecretRotation calls DELETE /api/v1/apps/{name}/secrets/{key}/rotation.
func (c *Client) DeleteSecretRotation(ctx context.Context, name, key string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/secrets/"+PathEscape(key)+"/rotation", nil, nil)
}

// RotateSecretNow calls POST .../secrets/{key}/rotation/rotate.
func (c *Client) RotateSecretNow(ctx context.Context, name, key string) (SecretRotationPolicyResource, error) {
	var out SecretRotationPolicyResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/secrets/"+PathEscape(key)+"/rotation/rotate", nil, &out)
	return out, err
}

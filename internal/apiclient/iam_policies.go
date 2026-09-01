package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// PolicyResource mirrors internal/api's policyResource
// (internal/api/iam_handlers.go).
type PolicyResource struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Document    json.RawMessage `json:"document"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// PolicyAttachmentResource mirrors internal/api's
// policyAttachmentResource.
type PolicyAttachmentResource struct {
	ID            string    `json:"id"`
	PolicyID      string    `json:"policy_id"`
	PrincipalType string    `json:"principal_type"`
	PrincipalID   string    `json:"principal_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// PolicyRequest mirrors internal/api's policyRequest, the body for both
// CreatePolicy and UpdatePolicy.
type PolicyRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Document    json.RawMessage `json:"document"`
}

// AttachPolicyRequest mirrors internal/api's attachPolicyRequest.
type AttachPolicyRequest struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
}

// CreatePolicy calls POST /api/v1/iam/policies.
func (c *Client) CreatePolicy(ctx context.Context, req PolicyRequest) (PolicyResource, error) {
	var out PolicyResource
	err := c.do(ctx, http.MethodPost, "/api/v1/iam/policies", req, &out)
	return out, err
}

// ListPolicies calls GET /api/v1/iam/policies.
func (c *Client) ListPolicies(ctx context.Context) ([]PolicyResource, error) {
	var out []PolicyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/iam/policies", nil, &out)
	return out, err
}

// GetPolicy calls GET /api/v1/iam/policies/{id}.
func (c *Client) GetPolicy(ctx context.Context, id string) (PolicyResource, error) {
	var out PolicyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/iam/policies/"+PathEscape(id), nil, &out)
	return out, err
}

// UpdatePolicy calls PUT /api/v1/iam/policies/{id}.
func (c *Client) UpdatePolicy(ctx context.Context, id string, req PolicyRequest) (PolicyResource, error) {
	var out PolicyResource
	err := c.do(ctx, http.MethodPut, "/api/v1/iam/policies/"+PathEscape(id), req, &out)
	return out, err
}

// DeletePolicy calls DELETE /api/v1/iam/policies/{id}.
func (c *Client) DeletePolicy(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/iam/policies/"+PathEscape(id), nil, nil)
}

// AttachPolicy calls POST /api/v1/iam/policies/{id}/attachments.
func (c *Client) AttachPolicy(ctx context.Context, id string, req AttachPolicyRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/iam/policies/"+PathEscape(id)+"/attachments", req, nil)
}

// DetachPolicy calls DELETE
// /api/v1/iam/policies/{id}/attachments/{principalType}/{principalID}.
func (c *Client) DetachPolicy(ctx context.Context, id, principalType, principalID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/iam/policies/"+PathEscape(id)+"/attachments/"+PathEscape(principalType)+"/"+PathEscape(principalID), nil, nil)
}

// ListPolicyAttachments calls GET /api/v1/iam/policies/{id}/attachments.
func (c *Client) ListPolicyAttachments(ctx context.Context, id string) ([]PolicyAttachmentResource, error) {
	var out []PolicyAttachmentResource
	err := c.do(ctx, http.MethodGet, "/api/v1/iam/policies/"+PathEscape(id)+"/attachments", nil, &out)
	return out, err
}

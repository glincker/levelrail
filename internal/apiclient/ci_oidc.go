package apiclient

import (
	"context"
	"net/http"
	"time"
)

// CITrust mirrors internal/api's ciTrustResource.
type CITrust struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	Issuer           string    `json:"issuer,omitempty"`
	Repository       string    `json:"repository"`
	RefPattern       string    `json:"ref_pattern"`
	Environment      string    `json:"environment"`
	ProtectedRefOnly bool      `json:"protected_ref_only,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// CITrustRequest is the body of POST /api/v1/apps/{name}/ci/trusts.
type CITrustRequest struct {
	Provider         string `json:"provider,omitempty"`
	Issuer           string `json:"issuer,omitempty"`
	OrgID            string `json:"org_id,omitempty"`
	Repository       string `json:"repository"`
	RefPattern       string `json:"ref_pattern"`
	Environment      string `json:"environment,omitempty"`
	ProtectedRefOnly bool   `json:"protected_ref_only,omitempty"`
}

// CIWorkflow mirrors internal/api's ciWorkflowResource.
type CIWorkflow struct {
	YAML        string `json:"yaml"`
	APIURL      string `json:"api_url"`
	HasTrust    bool   `json:"has_trust"`
	NeedsDomain bool   `json:"needs_dashboard_url"`
}

// CIToken is the short-lived token POST /api/v1/ci/oidc-exchange returns.
type CIToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ListCITrusts calls GET /api/v1/apps/{name}/ci/trusts.
func (c *Client) ListCITrusts(ctx context.Context, name string) ([]CITrust, error) {
	var out []CITrust
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/ci/trusts", nil, &out)
	return out, err
}

// CreateCITrust calls POST /api/v1/apps/{name}/ci/trusts.
func (c *Client) CreateCITrust(ctx context.Context, name string, req CITrustRequest) (CITrust, error) {
	var out CITrust
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/ci/trusts", req, &out)
	return out, err
}

// DeleteCITrust calls DELETE /api/v1/apps/{name}/ci/trusts/{id}.
func (c *Client) DeleteCITrust(ctx context.Context, name, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/ci/trusts/"+PathEscape(id), nil, nil)
}

// GetCIWorkflow calls GET /api/v1/apps/{name}/ci/workflow.
func (c *Client) GetCIWorkflow(ctx context.Context, name string) (CIWorkflow, error) {
	var out CIWorkflow
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/ci/workflow", nil, &out)
	return out, err
}

// ExchangeCIToken calls POST /api/v1/ci/oidc-exchange with a CI identity token.
func (c *Client) ExchangeCIToken(ctx context.Context, app, idToken string) (CIToken, error) {
	var out CIToken
	err := c.do(ctx, http.MethodPost, "/api/v1/ci/oidc-exchange", map[string]string{"app": app, "id_token": idToken}, &out)
	return out, err
}

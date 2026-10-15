package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PackageRegistryResource mirrors internal/api's packageRegistryResource.
type PackageRegistryResource struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Ecosystem      string    `json:"ecosystem"`
	URL            string    `json:"url"`
	Username       string    `json:"username,omitempty"`
	ModulePatterns string    `json:"module_patterns,omitempty"`
	ScopeKind      string    `json:"scope_kind"`
	ScopeRef       string    `json:"scope_ref,omitempty"`
	BuildSecretID  string    `json:"build_secret_id"`
	CreatedBy      string    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// CreatePackageRegistryRequest is POST /api/v1/package-registries' body.
type CreatePackageRegistryRequest struct {
	Name           string `json:"name"`
	Ecosystem      string `json:"ecosystem"`
	URL            string `json:"url"`
	Username       string `json:"username,omitempty"`
	Secret         string `json:"secret"`
	ModulePatterns string `json:"module_patterns,omitempty"`
	ScopeKind      string `json:"scope_kind,omitempty"`
	ScopeRef       string `json:"scope_ref,omitempty"`
}

// RegistryCredentialUsage mirrors the registry credential usage response.
type RegistryCredentialUsage struct {
	Services []string `json:"services"`
}

// ListPackageRegistries calls GET /api/v1/package-registries.
func (c *Client) ListPackageRegistries(ctx context.Context) ([]PackageRegistryResource, error) {
	var out []PackageRegistryResource
	err := c.do(ctx, http.MethodGet, "/api/v1/package-registries", nil, &out)
	return out, err
}

// CreatePackageRegistry calls POST /api/v1/package-registries.
func (c *Client) CreatePackageRegistry(ctx context.Context, req CreatePackageRegistryRequest) (PackageRegistryResource, error) {
	var out PackageRegistryResource
	err := c.do(ctx, http.MethodPost, "/api/v1/package-registries", req, &out)
	return out, err
}

// DeletePackageRegistry calls DELETE /api/v1/package-registries/{id}.
func (c *Client) DeletePackageRegistry(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/package-registries/"+PathEscape(id), nil, nil)
}

// TestPackageRegistry calls POST /api/v1/package-registries/{id}/test.
func (c *Client) TestPackageRegistry(ctx context.Context, id string) (NodeProviderTestResult, error) {
	var out NodeProviderTestResult
	err := c.do(ctx, http.MethodPost, "/api/v1/package-registries/"+PathEscape(id)+"/test", nil, &out)
	return out, err
}

// GetRegistryCredentialUsage calls GET /api/v1/registry-credentials/{id}/usage.
func (c *Client) GetRegistryCredentialUsage(ctx context.Context, id string) (RegistryCredentialUsage, error) {
	var out RegistryCredentialUsage
	err := c.do(ctx, http.MethodGet, "/api/v1/registry-credentials/"+PathEscape(id)+"/usage", nil, &out)
	return out, err
}

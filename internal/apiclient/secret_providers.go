package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// SecretProviderResource is one Doppler, Infisical or 1Password connection; the token is never returned.
type SecretProviderResource struct {
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Project     string     `json:"project"`
	Environment string     `json:"environment"`
	Path        string     `json:"path,omitempty"`
	BaseURL     string     `json:"base_url,omitempty"`
	SelfHosted  bool       `json:"self_hosted"`
	HasToken    bool       `json:"has_token"`
	ClientID    string     `json:"client_id,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	GraceUntil  *time.Time `json:"rotation_grace_until,omitempty"`
}

// PutSecretProviderRequest creates or updates a provider; an empty Token keeps the stored one.
type PutSecretProviderRequest struct {
	Type        string `json:"type"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Path        string `json:"path,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Token       string `json:"token,omitempty"`
}

// SecretProviderTestResult is the outcome of a live connection test.
type SecretProviderTestResult struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// ListSecretProviders calls GET /api/v1/settings/secret-providers.
func (c *Client) ListSecretProviders(ctx context.Context) ([]SecretProviderResource, error) {
	var out struct {
		Providers []SecretProviderResource `json:"providers"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/secret-providers", nil, &out)
	return out.Providers, err
}

// PutSecretProvider calls PUT /api/v1/settings/secret-providers/{name}.
func (c *Client) PutSecretProvider(ctx context.Context, name string, req PutSecretProviderRequest) (SecretProviderResource, error) {
	var out SecretProviderResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/secret-providers/"+url.PathEscape(name), req, &out)
	return out, err
}

// DeleteSecretProvider calls DELETE /api/v1/settings/secret-providers/{name}.
func (c *Client) DeleteSecretProvider(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/settings/secret-providers/"+url.PathEscape(name), nil, nil)
}

// TestSecretProvider calls POST /api/v1/settings/secret-providers/{name}/test.
func (c *Client) TestSecretProvider(ctx context.Context, name string) (SecretProviderTestResult, error) {
	var out SecretProviderTestResult
	err := c.do(ctx, http.MethodPost, "/api/v1/settings/secret-providers/"+url.PathEscape(name)+"/test", nil, &out)
	return out, err
}

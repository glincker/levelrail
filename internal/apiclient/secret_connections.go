package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// SecretConnectionConfig is the non-secret part of a connection.
type SecretConnectionConfig struct {
	Address     string `json:"address,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Mount       string `json:"mount,omitempty"`
	AuthMethod  string `json:"auth_method,omitempty"`
	RoleID      string `json:"role_id,omitempty"`
	Project     string `json:"project,omitempty"`
	Environment string `json:"environment,omitempty"`
	Path        string `json:"path,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
}

// SecretConnection is one named secret connection; the credential is never returned.
type SecretConnection struct {
	Name             string                 `json:"name"`
	Type             string                 `json:"type"`
	Legacy           bool                   `json:"legacy"`
	ManagedBy        string                 `json:"managed_by,omitempty"`
	Config           SecretConnectionConfig `json:"config"`
	HasCredential    bool                   `json:"has_credential"`
	CredentialSource string                 `json:"credential_source,omitempty"`
	AllowedPrefixes  []string               `json:"allowed_prefixes,omitempty"`
	AllowedEnvs      []string               `json:"allowed_envs,omitempty"`
	CreatedBy        string                 `json:"created_by,omitempty"`
	UpdatedAt        *time.Time             `json:"updated_at,omitempty"`
}

// SecretConnectionRequest creates or updates a connection. Credential is write-only.
type SecretConnectionRequest struct {
	Name          string                 `json:"name,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Credential    string                 `json:"credential,omitempty"`
	CredentialRef string                 `json:"credential_ref,omitempty"`
	Config        SecretConnectionConfig `json:"config"`
	// A non nil list replaces that allow list, an empty one clears it; nil keeps it.
	AllowedPrefixes *[]string `json:"allowed_prefixes,omitempty"`
	AllowedEnvs     *[]string `json:"allowed_envs,omitempty"`
}

// SecretConnectionTest is the outcome of a read-only health probe.
type SecretConnectionTest struct {
	OK     bool   `json:"ok"`
	Class  string `json:"class"`
	Reason string `json:"reason,omitempty"`
}

const secretConnectionsPath = "/api/v1/settings/secret-connections" //nolint:gosec // route and column names, not credentials

// ListSecretConnections calls GET /api/v1/settings/secret-connections.
func (c *Client) ListSecretConnections(ctx context.Context) ([]SecretConnection, error) {
	var out struct {
		Connections []SecretConnection `json:"connections"`
	}
	err := c.do(ctx, http.MethodGet, secretConnectionsPath, nil, &out)
	return out.Connections, err
}

// CreateSecretConnection calls POST /api/v1/settings/secret-connections.
func (c *Client) CreateSecretConnection(ctx context.Context, req SecretConnectionRequest) (SecretConnection, error) {
	var out SecretConnection
	err := c.do(ctx, http.MethodPost, secretConnectionsPath, req, &out)
	return out, err
}

// UpdateSecretConnection calls PUT /api/v1/settings/secret-connections/{name}.
func (c *Client) UpdateSecretConnection(ctx context.Context, name string, req SecretConnectionRequest) (SecretConnection, error) {
	var out SecretConnection
	err := c.do(ctx, http.MethodPut, secretConnectionsPath+"/"+url.PathEscape(name), req, &out)
	return out, err
}

// DeleteSecretConnection calls DELETE /api/v1/settings/secret-connections/{name}.
func (c *Client) DeleteSecretConnection(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, secretConnectionsPath+"/"+url.PathEscape(name), nil, nil)
}

// TestSecretConnection calls POST /api/v1/settings/secret-connections/{name}/test.
// An empty path proves login only; a path adds a read-only access check.
func (c *Client) TestSecretConnection(ctx context.Context, name, path string) (SecretConnectionTest, error) {
	var out SecretConnectionTest
	var body any
	if path != "" {
		body = map[string]string{"path": path}
	}
	err := c.do(ctx, http.MethodPost, secretConnectionsPath+"/"+url.PathEscape(name)+"/test", body, &out)
	return out, err
}

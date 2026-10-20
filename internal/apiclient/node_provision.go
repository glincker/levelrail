package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// NodeProviderResource mirrors internal/api's nodeProviderResource.
type NodeProviderResource struct {
	Provider       string     `json:"provider"`
	HasToken       bool       `json:"has_token"`
	Status         string     `json:"status,omitempty"`
	Methods        []string   `json:"methods,omitempty"`
	Method         string     `json:"method,omitempty"`
	Label          string     `json:"label,omitempty"`
	Location       string     `json:"location,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	ConnectedBy    string     `json:"connected_by,omitempty"`
	ConnectedAt    *time.Time `json:"connected_at,omitempty"`
	LastTestedAt   *time.Time `json:"last_tested_at,omitempty"`
	LastTestOK     *bool      `json:"last_test_ok,omitempty"`
	LastTestDetail string     `json:"last_test_detail,omitempty"`
}

// NodeProviderTestResult mirrors internal/api's nodeProviderTestResult.
type NodeProviderTestResult struct {
	OK       bool      `json:"ok"`
	Message  string    `json:"message"`
	TestedAt time.Time `json:"tested_at"`
}

// NodeProviderRegionResource mirrors internal/api's
// nodeProviderRegionResource.
type NodeProviderRegionResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NodeProviderSizeResource mirrors internal/api's nodeProviderSizeResource.
type NodeProviderSizeResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	VCPUs        int    `json:"vcpus"`
	MemoryMB     int    `json:"memory_mb"`
	DiskGB       int    `json:"disk_gb"`
	PriceMonthly string `json:"price_monthly,omitempty"`
	Currency     string `json:"currency,omitempty"`
}

// NodeProvisionResource mirrors internal/api's nodeProvisionResource.
type NodeProvisionResource struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	Region        string    `json:"region"`
	Size          string    `json:"size"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	IPAddress     string    `json:"ip_address,omitempty"`
	NodeID        string    `json:"node_id,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SetNodeProviderCredentialRequest is POST /api/v1/node-providers' body.
// The fields after Token mirror internal/api's own request struct and
// are aws-only: for hetzner/digitalocean, Token alone is the provider's
// bearer API token.
type SetNodeProviderCredentialRequest struct {
	Provider              string `json:"provider"`
	Token                 string `json:"token"`
	SecretAccessKey       string `json:"secret_access_key,omitempty"`
	SessionToken          string `json:"session_token,omitempty"`
	Region                string `json:"region,omitempty"`
	RoleARN               string `json:"role_arn,omitempty"`
	UseAmbientCredentials bool   `json:"use_ambient_credentials,omitempty"`
	Method                string `json:"method,omitempty"`
	Verify                bool   `json:"verify,omitempty"`
	Label                 string `json:"label,omitempty"`
	Location              string `json:"location,omitempty"`
	Notes                 string `json:"notes,omitempty"`
}

// CreateNodeProvisionRequest is POST /api/v1/nodes/provision's body.
type CreateNodeProvisionRequest struct {
	Provider         string `json:"provider"`
	Region           string `json:"region"`
	Size             string `json:"size"`
	Name             string `json:"name"`
	Role             string `json:"role,omitempty"`
	ControlPlaneAddr string `json:"control_plane_addr"`
	// AllowSSHInbound is aws-only; other providers ignore it. Off by
	// default, matching this platform's no-inbound-ports architecture.
	AllowSSHInbound bool `json:"allow_ssh_inbound,omitempty"`
}

func nodeProvidersPath() string { return "/api/v1/node-providers" }

// ListNodeProviders calls GET /api/v1/node-providers.
func (c *Client) ListNodeProviders(ctx context.Context) ([]NodeProviderResource, error) {
	var out []NodeProviderResource
	err := c.do(ctx, http.MethodGet, nodeProvidersPath(), nil, &out)
	return out, err
}

// SetNodeProviderCredential calls POST /api/v1/node-providers.
func (c *Client) SetNodeProviderCredential(ctx context.Context, req SetNodeProviderCredentialRequest) (NodeProviderResource, error) {
	var out NodeProviderResource
	err := c.do(ctx, http.MethodPost, nodeProvidersPath(), req, &out)
	return out, err
}

// TestNodeProvider calls POST /api/v1/node-providers/{provider}/test with
// no body, testing the stored credential.
func (c *Client) TestNodeProvider(ctx context.Context, provider string) (NodeProviderTestResult, error) {
	var out NodeProviderTestResult
	err := c.do(ctx, http.MethodPost, nodeProvidersPath()+"/"+PathEscape(provider)+"/test", nil, &out)
	return out, err
}

// DisconnectNodeProvider calls DELETE /api/v1/node-providers/{provider}.
func (c *Client) DisconnectNodeProvider(ctx context.Context, provider string) error {
	return c.do(ctx, http.MethodDelete, nodeProvidersPath()+"/"+PathEscape(provider), nil, nil)
}

// ListNodeProviderRegions calls GET /api/v1/node-providers/{provider}/regions.
func (c *Client) ListNodeProviderRegions(ctx context.Context, provider string) ([]NodeProviderRegionResource, error) {
	var out []NodeProviderRegionResource
	err := c.do(ctx, http.MethodGet, nodeProvidersPath()+"/"+PathEscape(provider)+"/regions", nil, &out)
	return out, err
}

// ListNodeProviderSizes calls GET
// /api/v1/node-providers/{provider}/sizes?region=X. region may be empty
// to list every size regardless of region.
func (c *Client) ListNodeProviderSizes(ctx context.Context, provider, region string) ([]NodeProviderSizeResource, error) {
	path := nodeProvidersPath() + "/" + PathEscape(provider) + "/sizes"
	if region != "" {
		path += "?region=" + url.QueryEscape(region)
	}
	var out []NodeProviderSizeResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func nodeProvisionsPath() string { return "/api/v1/node-provisions" }

// CreateNodeProvision calls POST /api/v1/nodes/provision.
func (c *Client) CreateNodeProvision(ctx context.Context, req CreateNodeProvisionRequest) (NodeProvisionResource, error) {
	var out NodeProvisionResource
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes/provision", req, &out)
	return out, err
}

// ListNodeProvisions calls GET /api/v1/node-provisions.
func (c *Client) ListNodeProvisions(ctx context.Context) ([]NodeProvisionResource, error) {
	var out []NodeProvisionResource
	err := c.do(ctx, http.MethodGet, nodeProvisionsPath(), nil, &out)
	return out, err
}

// GetNodeProvision calls GET /api/v1/node-provisions/{id}.
func (c *Client) GetNodeProvision(ctx context.Context, id string) (NodeProvisionResource, error) {
	var out NodeProvisionResource
	err := c.do(ctx, http.MethodGet, nodeProvisionsPath()+"/"+PathEscape(id), nil, &out)
	return out, err
}

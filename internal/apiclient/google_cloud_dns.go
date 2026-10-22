package apiclient

import (
	"context"
	"net/http"
)

const googleCloudDNSSettingsPath = "/api/v1/settings/google-cloud-dns"

// GoogleCloudDNSResource mirrors internal/api's googleCloudDNSResource. The
// service account JSON never appears here.
type GoogleCloudDNSResource struct {
	Enabled     bool   `json:"enabled"`
	ProjectID   string `json:"project_id,omitempty"`
	ClientEmail string `json:"client_email,omitempty"`
	HasKey      bool   `json:"has_key"`
}

// UpdateGoogleCloudDNSRequest mirrors internal/api's updateGoogleCloudDNSRequest.
// An empty ServiceAccountJSON keeps the stored key.
type UpdateGoogleCloudDNSRequest struct {
	Enabled            bool   `json:"enabled"`
	ServiceAccountJSON string `json:"service_account_json,omitempty"`
}

// GoogleCloudDNSTestResult mirrors internal/api's googleCloudDNSTestResponse.
type GoogleCloudDNSTestResult struct {
	OK    bool     `json:"ok"`
	Zones []string `json:"zones,omitempty"`
	Error string   `json:"error,omitempty"`
}

// GetGoogleCloudDNS calls GET /api/v1/settings/google-cloud-dns.
func (c *Client) GetGoogleCloudDNS(ctx context.Context) (GoogleCloudDNSResource, error) {
	var out GoogleCloudDNSResource
	err := c.do(ctx, http.MethodGet, googleCloudDNSSettingsPath, nil, &out)
	return out, err
}

// SetGoogleCloudDNS calls PUT /api/v1/settings/google-cloud-dns.
func (c *Client) SetGoogleCloudDNS(ctx context.Context, req UpdateGoogleCloudDNSRequest) (GoogleCloudDNSResource, error) {
	var out GoogleCloudDNSResource
	err := c.do(ctx, http.MethodPut, googleCloudDNSSettingsPath, req, &out)
	return out, err
}

// DisconnectGoogleCloudDNS calls DELETE /api/v1/settings/google-cloud-dns.
func (c *Client) DisconnectGoogleCloudDNS(ctx context.Context) (GoogleCloudDNSResource, error) {
	var out GoogleCloudDNSResource
	err := c.do(ctx, http.MethodDelete, googleCloudDNSSettingsPath, nil, &out)
	return out, err
}

// TestGoogleCloudDNS calls POST /api/v1/settings/google-cloud-dns/test.
func (c *Client) TestGoogleCloudDNS(ctx context.Context) (GoogleCloudDNSTestResult, error) {
	var out GoogleCloudDNSTestResult
	err := c.do(ctx, http.MethodPost, googleCloudDNSSettingsPath+"/test", nil, &out)
	return out, err
}

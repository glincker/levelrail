package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// CloudConnectionConfig is the non-secret part of a cloud connection.
type CloudConnectionConfig struct {
	Region         string `json:"region,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	Compartment    string `json:"compartment,omitempty"`
	RoleARN        string `json:"role_arn,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
	ResourceGroup  string `json:"resource_group,omitempty"`
	OIDCAudience   string `json:"oidc_audience,omitempty"`
}

// CloudConnection is one named cloud account (preview). It holds a credential reference, never a credential.
type CloudConnection struct {
	Name           string                `json:"name"`
	Provider       string                `json:"provider"`
	TeamID         string                `json:"team_id,omitempty"`
	AuthMethod     string                `json:"auth_method"`
	Config         CloudConnectionConfig `json:"config"`
	CredentialRef  string                `json:"credential_ref,omitempty"`
	Preview        bool                  `json:"preview"`
	CreatedBy      string                `json:"created_by,omitempty"`
	UpdatedAt      *time.Time            `json:"updated_at,omitempty"`
	LastCheckedAt  *time.Time            `json:"last_checked_at,omitempty"`
	LastCheckClass string                `json:"last_check_class,omitempty"`
}

// CloudConnectionRequest creates or updates a cloud connection; on update empty fields keep the stored value.
type CloudConnectionRequest struct {
	Name          string                `json:"name,omitempty"`
	Provider      string                `json:"provider,omitempty"`
	TeamID        string                `json:"team_id,omitempty"`
	AuthMethod    string                `json:"auth_method,omitempty"`
	CredentialRef string                `json:"credential_ref,omitempty"`
	Config        CloudConnectionConfig `json:"config"`
}

// CloudConnectionTest is the outcome of a read-only health probe.
type CloudConnectionTest struct {
	OK     bool   `json:"ok"`
	Class  string `json:"class"`
	Reason string `json:"reason,omitempty"`
}

const cloudConnectionsPath = "/api/v1/settings/cloud-connections"

// ListCloudConnections calls GET /api/v1/settings/cloud-connections.
func (c *Client) ListCloudConnections(ctx context.Context) ([]CloudConnection, error) {
	var out struct {
		Connections []CloudConnection `json:"connections"`
	}
	err := c.do(ctx, http.MethodGet, cloudConnectionsPath, nil, &out)
	return out.Connections, err
}

// CreateCloudConnection calls POST /api/v1/settings/cloud-connections.
func (c *Client) CreateCloudConnection(ctx context.Context, req CloudConnectionRequest) (CloudConnection, error) {
	var out CloudConnection
	err := c.do(ctx, http.MethodPost, cloudConnectionsPath, req, &out)
	return out, err
}

// UpdateCloudConnection calls PUT /api/v1/settings/cloud-connections/{name}.
func (c *Client) UpdateCloudConnection(ctx context.Context, name string, req CloudConnectionRequest) (CloudConnection, error) {
	var out CloudConnection
	err := c.do(ctx, http.MethodPut, cloudConnectionsPath+"/"+url.PathEscape(name), req, &out)
	return out, err
}

// DeleteCloudConnection calls DELETE /api/v1/settings/cloud-connections/{name}.
func (c *Client) DeleteCloudConnection(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, cloudConnectionsPath+"/"+url.PathEscape(name), nil, nil)
}

// TestCloudConnection calls POST /api/v1/settings/cloud-connections/{name}/test.
func (c *Client) TestCloudConnection(ctx context.Context, name string) (CloudConnectionTest, error) {
	var out CloudConnectionTest
	err := c.do(ctx, http.MethodPost, cloudConnectionsPath+"/"+url.PathEscape(name)+"/test", nil, &out)
	return out, err
}

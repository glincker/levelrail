package apiclient

import (
	"context"
	"net/http"
)

// SAMLCertificateInfo summarizes one trusted IDP signing certificate.
type SAMLCertificateInfo struct {
	Subject  string `json:"subject"`
	NotAfter string `json:"not_after"`
}

// SAMLConnection mirrors internal/api's samlConnectionResource.
type SAMLConnection struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Enabled         bool                  `json:"enabled"`
	Configured      bool                  `json:"configured"`
	IDPEntityID     string                `json:"idp_entity_id"`
	IDPSSOURL       string                `json:"idp_sso_url"`
	IDPCertificates string                `json:"idp_certificates"`
	IDPMetadataURL  string                `json:"idp_metadata_url"`
	Certificates    []SAMLCertificateInfo `json:"certificates"`
	EmailAttribute  string                `json:"email_attribute"`
	NameAttribute   string                `json:"name_attribute"`
	GroupsAttribute string                `json:"groups_attribute"`
	AllowedDomains  []string              `json:"allowed_domains"`
	JITProvisioning bool                  `json:"jit_provisioning"`
	DefaultRoleID   string                `json:"default_role_id"`
	SyncRoles       bool                  `json:"sync_roles"`
	SPEntityID      string                `json:"sp_entity_id"`
	SPACSURL        string                `json:"sp_acs_url"`
	SPMetadataURL   string                `json:"sp_metadata_url"`
	CreatedAt       string                `json:"created_at"`
	UpdatedAt       string                `json:"updated_at"`
}

// SAMLConnectionRequest is the full body of a create or update.
type SAMLConnectionRequest struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	IDPEntityID     string   `json:"idp_entity_id"`
	IDPSSOURL       string   `json:"idp_sso_url"`
	IDPCertificates string   `json:"idp_certificates"`
	IDPMetadataURL  string   `json:"idp_metadata_url"`
	EmailAttribute  string   `json:"email_attribute"`
	NameAttribute   string   `json:"name_attribute"`
	GroupsAttribute string   `json:"groups_attribute"`
	AllowedDomains  []string `json:"allowed_domains"`
	JITProvisioning bool     `json:"jit_provisioning"`
	DefaultRoleID   string   `json:"default_role_id"`
	SyncRoles       bool     `json:"sync_roles"`
}

// RequestFromSAMLConnection copies the editable fields, for read-modify-write updates.
func RequestFromSAMLConnection(c SAMLConnection) SAMLConnectionRequest {
	return SAMLConnectionRequest{
		Name: c.Name, Enabled: c.Enabled, IDPEntityID: c.IDPEntityID, IDPSSOURL: c.IDPSSOURL,
		IDPCertificates: c.IDPCertificates, IDPMetadataURL: c.IDPMetadataURL, EmailAttribute: c.EmailAttribute,
		NameAttribute: c.NameAttribute, GroupsAttribute: c.GroupsAttribute, AllowedDomains: c.AllowedDomains,
		JITProvisioning: c.JITProvisioning, DefaultRoleID: c.DefaultRoleID, SyncRoles: c.SyncRoles,
	}
}

// SAMLMetadataImport carries IDP metadata by URL or inline XML (exactly one).
type SAMLMetadataImport struct {
	URL string `json:"url,omitempty"`
	XML string `json:"xml,omitempty"`
}

// SSOGroupMapping maps a directory group to a stored role.
type SSOGroupMapping struct {
	Group    string `json:"group"`
	RoleID   string `json:"role_id"`
	Priority int    `json:"priority"`
}

// SSOGroupMappings is the full mapping set.
type SSOGroupMappings struct {
	Mappings []SSOGroupMapping `json:"mappings"`
}

// SSOSettings mirrors internal/api's ssoSettingsResource.
type SSOSettings struct {
	EnforceSSO       bool   `json:"enforce_sso"`
	SPCertificate    string `json:"sp_certificate"`
	BaseURL          string `json:"base_url"`
	BaseURLError     string `json:"base_url_error,omitempty"`
	MasterKeyMissing bool   `json:"master_key_missing"`
	SCIMBaseURL      string `json:"scim_base_url"`
	SCIMDefaultRole  string `json:"scim_default_role_id"`
}

// ListSAMLConnections calls GET /api/v1/sso/saml/connections.
func (c *Client) ListSAMLConnections(ctx context.Context) ([]SAMLConnection, error) {
	var out []SAMLConnection
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/saml/connections", nil, &out)
	return out, err
}

// GetSAMLConnection calls GET /api/v1/sso/saml/connections/{id}.
func (c *Client) GetSAMLConnection(ctx context.Context, id string) (SAMLConnection, error) {
	var out SAMLConnection
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/saml/connections/"+PathEscape(id), nil, &out)
	return out, err
}

// CreateSAMLConnection calls POST /api/v1/sso/saml/connections.
func (c *Client) CreateSAMLConnection(ctx context.Context, req SAMLConnectionRequest) (SAMLConnection, error) {
	var out SAMLConnection
	err := c.do(ctx, http.MethodPost, "/api/v1/sso/saml/connections", req, &out)
	return out, err
}

// UpdateSAMLConnection calls PUT /api/v1/sso/saml/connections/{id}.
func (c *Client) UpdateSAMLConnection(ctx context.Context, id string, req SAMLConnectionRequest) (SAMLConnection, error) {
	var out SAMLConnection
	err := c.do(ctx, http.MethodPut, "/api/v1/sso/saml/connections/"+PathEscape(id), req, &out)
	return out, err
}

// DeleteSAMLConnection calls DELETE /api/v1/sso/saml/connections/{id}.
func (c *Client) DeleteSAMLConnection(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/sso/saml/connections/"+PathEscape(id), nil, nil)
}

// ImportSAMLMetadata calls POST /api/v1/sso/saml/connections/{id}/metadata.
func (c *Client) ImportSAMLMetadata(ctx context.Context, id string, req SAMLMetadataImport) (SAMLConnection, error) {
	var out SAMLConnection
	err := c.do(ctx, http.MethodPost, "/api/v1/sso/saml/connections/"+PathEscape(id)+"/metadata", req, &out)
	return out, err
}

// URL returns an absolute URL on the API target, for pages the operator opens in a browser.
func (c *Client) URL(path string) string { return c.baseURL + path }

// SAMLSPMetadata fetches the public SP metadata XML for a connection.
func (c *Client) SAMLSPMetadata(ctx context.Context, id string) ([]byte, error) {
	return c.downloadRaw(ctx, "/api/v1/auth/saml/"+PathEscape(id)+"/metadata")
}

// ListSSOGroupMappings calls GET /api/v1/sso/group-mappings.
func (c *Client) ListSSOGroupMappings(ctx context.Context) (SSOGroupMappings, error) {
	var out SSOGroupMappings
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/group-mappings", nil, &out)
	return out, err
}

// ReplaceSSOGroupMappings calls PUT /api/v1/sso/group-mappings.
func (c *Client) ReplaceSSOGroupMappings(ctx context.Context, req SSOGroupMappings) (SSOGroupMappings, error) {
	var out SSOGroupMappings
	err := c.do(ctx, http.MethodPut, "/api/v1/sso/group-mappings", req, &out)
	return out, err
}

// GetSSOSettings calls GET /api/v1/sso/settings.
func (c *Client) GetSSOSettings(ctx context.Context) (SSOSettings, error) {
	var out SSOSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/settings", nil, &out)
	return out, err
}

// SetSSOEnforce calls PUT /api/v1/sso/settings.
func (c *Client) SetSSOEnforce(ctx context.Context, on bool) (SSOSettings, error) {
	var out SSOSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/sso/settings", map[string]bool{"enforce_sso": on}, &out)
	return out, err
}

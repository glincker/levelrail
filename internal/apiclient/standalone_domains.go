package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// StandaloneScope is the {name} the per-domain endpoints take for a
// standalone domain, e.g. /api/v1/apps/_standalone/domains/x/waf.
const StandaloneScope = "_standalone"

// StandaloneDomain mirrors internal/api's standaloneDomainResource.
type StandaloneDomain struct {
	Domain             string `json:"domain"`
	Kind               string `json:"kind"`
	Scope              string `json:"scope"`
	Target             string `json:"target"`
	TargetKind         string `json:"target_kind"`
	TargetURL          string `json:"target_url"`
	UpstreamTLS        bool   `json:"upstream_tls"`
	TLSSkipVerify      bool   `json:"tls_skip_verify"`
	TLSServerName      string `json:"tls_server_name,omitempty"`
	HostHeader         string `json:"host_header"`
	WebSocket          bool   `json:"websocket"`
	DialTimeoutSeconds int    `json:"dial_timeout_seconds"`
	ReadTimeoutSeconds int    `json:"read_timeout_seconds"`
	StripPrefix        string `json:"strip_prefix,omitempty"`
	Description        string `json:"description,omitempty"`
	CreatedBy          string `json:"created_by,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// StandaloneDomainRequest is POST and PUT's body; nil fields keep the stored value on PUT.
type StandaloneDomainRequest struct {
	Domain             string  `json:"domain,omitempty"`
	Target             *string `json:"target,omitempty"`
	UpstreamTLS        *bool   `json:"upstream_tls,omitempty"`
	TLSSkipVerify      *bool   `json:"tls_skip_verify,omitempty"`
	TLSServerName      *string `json:"tls_server_name,omitempty"`
	HostHeader         *string `json:"host_header,omitempty"`
	WebSocket          *bool   `json:"websocket,omitempty"`
	DialTimeoutSeconds *int    `json:"dial_timeout_seconds,omitempty"`
	ReadTimeoutSeconds *int    `json:"read_timeout_seconds,omitempty"`
	StripPrefix        *string `json:"strip_prefix,omitempty"`
	Description        *string `json:"description,omitempty"`
	Confirm            bool    `json:"confirm,omitempty"`
}

// ListStandaloneDomains calls GET /api/v1/domains/standalone.
func (c *Client) ListStandaloneDomains(ctx context.Context) ([]StandaloneDomain, error) {
	var out []StandaloneDomain
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/standalone", nil, &out)
	return out, err
}

// CreateStandaloneDomain calls POST /api/v1/domains/standalone.
func (c *Client) CreateStandaloneDomain(ctx context.Context, req StandaloneDomainRequest) (StandaloneDomain, error) {
	var out StandaloneDomain
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/standalone", req, &out)
	return out, err
}

// UpdateStandaloneDomain calls PUT /api/v1/domains/standalone/{domain}.
func (c *Client) UpdateStandaloneDomain(ctx context.Context, domain string, req StandaloneDomainRequest) (StandaloneDomain, error) {
	var out StandaloneDomain
	err := c.do(ctx, http.MethodPut, "/api/v1/domains/standalone/"+url.PathEscape(domain), req, &out)
	return out, err
}

// DeleteStandaloneDomain calls DELETE /api/v1/domains/standalone/{domain}.
func (c *Client) DeleteStandaloneDomain(ctx context.Context, domain string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/domains/standalone/"+url.PathEscape(domain), nil, nil)
}

// DNSRecordSuggestion is one record to create so a domain reaches this server.
type DNSRecordSuggestion struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Alternative bool   `json:"alternative,omitempty"`
}

// ServerAddressPair is an IPv4 and IPv6 address, either may be empty.
type ServerAddressPair struct {
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
}

// ServerAddress mirrors internal/api's serverAddressResource.
type ServerAddress struct {
	IPv4                  string                `json:"ipv4,omitempty"`
	IPv6                  string                `json:"ipv6,omitempty"`
	Source                string                `json:"source"`
	Detected              ServerAddressPair     `json:"detected"`
	Override              ServerAddressPair     `json:"override"`
	PublicHost            string                `json:"public_host,omitempty"`
	PublicHostSource      string                `json:"public_host_source,omitempty"`
	BehindNAT             bool                  `json:"behind_nat"`
	TLSTerminatedUpstream bool                  `json:"tls_terminated_upstream"`
	AppsBaseDomain        string                `json:"apps_base_domain,omitempty"`
	Domain                string                `json:"domain,omitempty"`
	Records               []DNSRecordSuggestion `json:"records,omitempty"`
}

// GetServerAddress calls GET /api/v1/system/server-address; domain may be empty.
func (c *Client) GetServerAddress(ctx context.Context, domain string) (ServerAddress, error) {
	path := "/api/v1/system/server-address"
	if domain != "" {
		path += "?domain=" + url.QueryEscape(domain)
	}
	var out ServerAddress
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// SetServerAddress calls PUT /api/v1/system/server-address; both empty clears the override.
func (c *Client) SetServerAddress(ctx context.Context, override ServerAddressPair) (ServerAddress, error) {
	var out ServerAddress
	err := c.do(ctx, http.MethodPut, "/api/v1/system/server-address", map[string]string{"ipv4": override.IPv4, "ipv6": override.IPv6}, &out)
	return out, err
}

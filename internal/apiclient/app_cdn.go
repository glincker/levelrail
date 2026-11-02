package apiclient

import (
	"context"
	"net/http"
)

// AppCDN is GET/PUT /api/v1/apps/{name}/cdn.
type AppCDN struct {
	App           string `json:"app"`
	Proxied       *bool  `json:"proxied"`
	GlobalProxied bool   `json:"global_proxied"`
	Effective     bool   `json:"effective"`
	SSLMode       string `json:"ssl_mode,omitempty"`
	SSLWarning    string `json:"ssl_warning,omitempty"`
}

// AppCDNResyncRecord is one DNS record's re-sync outcome.
type AppCDNResyncRecord struct {
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

// AppCDNResync is the answer to POST /api/v1/apps/{name}/cdn/resync.
type AppCDNResync struct {
	App       string               `json:"app"`
	Effective bool                 `json:"effective"`
	DryRun    bool                 `json:"dry_run"`
	Updated   int                  `json:"updated"`
	Failed    int                  `json:"failed"`
	Records   []AppCDNResyncRecord `json:"records"`
}

func appCDNPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/cdn"
}

// GetAppCDN calls GET /api/v1/apps/{name}/cdn.
func (c *Client) GetAppCDN(ctx context.Context, app string) (AppCDN, error) {
	var out AppCDN
	err := c.do(ctx, http.MethodGet, appCDNPath(app), nil, &out)
	return out, err
}

// SetAppCDN calls PUT /api/v1/apps/{name}/cdn; a nil proxied clears the override.
func (c *Client) SetAppCDN(ctx context.Context, app string, proxied *bool) (AppCDN, error) {
	var out AppCDN
	err := c.do(ctx, http.MethodPut, appCDNPath(app), struct {
		Proxied *bool `json:"proxied"`
	}{proxied}, &out)
	return out, err
}

// ResyncAppCDN calls POST /api/v1/apps/{name}/cdn/resync.
func (c *Client) ResyncAppCDN(ctx context.Context, app string, dryRun bool) (AppCDNResync, error) {
	var out AppCDNResync
	err := c.do(ctx, http.MethodPost, appCDNPath(app)+"/resync", struct {
		DryRun bool `json:"dry_run"`
	}{dryRun}, &out)
	return out, err
}

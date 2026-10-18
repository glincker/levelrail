package apiclient

import (
	"context"
	"net/http"
)

// EnvScanFinding is one suspected credential in scanned env values. It never carries the value.
type EnvScanFinding struct {
	Var         string `json:"var"`
	RuleID      string `json:"rule_id"`
	Description string `json:"description"`
	Fingerprint string `json:"fingerprint"`
	Allowed     bool   `json:"allowed,omitempty"`
}

// EnvScanResponse is POST .../env/scan's body.
type EnvScanResponse struct {
	Findings  []EnvScanFinding `json:"findings"`
	Truncated bool             `json:"truncated"`
}

// DatabaseLink is one app a database injects a connection env var into.
type DatabaseLink struct {
	App    string `json:"app"`
	EnvVar string `json:"env_var"`
	Field  string `json:"field"`
}

// ScanAppEnv calls POST /api/v1/apps/{name}/env/scan.
func (c *Client) ScanAppEnv(ctx context.Context, name string, vars map[string]string) (EnvScanResponse, error) {
	var out EnvScanResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/env/scan", map[string]map[string]string{"vars": vars}, &out)
	return out, err
}

// ListDatabaseLinks calls GET /api/v1/databases/{name}/links.
func (c *Client) ListDatabaseLinks(ctx context.Context, database string) ([]DatabaseLink, error) {
	var out struct {
		Links []DatabaseLink `json:"links"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(database)+"/links", nil, &out)
	return out.Links, err
}

// ConnectDatabaseApp calls POST /api/v1/databases/{name}/links.
func (c *Client) ConnectDatabaseApp(ctx context.Context, database, app, envVar, field string) (AppConnectionResource, error) {
	var out AppConnectionResource
	body := map[string]string{"app": app, "env_var": envVar, "field": field}
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(database)+"/links", body, &out)
	return out, err
}

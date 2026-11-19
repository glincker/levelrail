package apiclient

import (
	"context"
	"net/http"
	"strings"
)

// StaticSiteDeployInfo is the newest deploy of a Pages site.
type StaticSiteDeployInfo struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// StaticSiteDetail mirrors internal/api's staticSiteDetail (GET /api/v1/static-sites/{name}).
type StaticSiteDetail struct {
	Name         string                `json:"name"`
	BuildType    string                `json:"build_type"`
	Domains      []string              `json:"domains"`
	Published    bool                  `json:"published"`
	FileCount    int                   `json:"file_count"`
	SizeBytes    int64                 `json:"size_bytes"`
	UpdatedAt    string                `json:"updated_at,omitempty"`
	LatestDeploy *StaticSiteDeployInfo `json:"latest_deploy,omitempty"`
}

// StaticSiteDomainRecord is one DNS record to create for a site domain.
type StaticSiteDomainRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// StaticSiteDomain mirrors the add and status responses of the site domain endpoints.
type StaticSiteDomain struct {
	Domain   string                   `json:"domain"`
	Mode     string                   `json:"mode"`
	Records  []StaticSiteDomainRecord `json:"records"`
	Verified bool                     `json:"verified"`
	Cert     string                   `json:"cert"`
	Detail   string                   `json:"detail,omitempty"`
}

// CreateStaticSite calls POST /api/v1/static-sites.
func (c *Client) CreateStaticSite(ctx context.Context, name string, domains []string) (StaticSiteDetail, error) {
	var out StaticSiteDetail
	body := map[string]any{"name": name}
	if len(domains) > 0 {
		body["domains"] = domains
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/static-sites", body, &out)
	return out, err
}

// GetStaticSite calls GET /api/v1/static-sites/{name}.
func (c *Client) GetStaticSite(ctx context.Context, name string) (StaticSiteDetail, error) {
	var out StaticSiteDetail
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name), nil, &out)
	return out, err
}

// AddStaticSiteDomain calls POST /api/v1/static-sites/{name}/domains.
func (c *Client) AddStaticSiteDomain(ctx context.Context, name, domain string) (StaticSiteDomain, error) {
	var out StaticSiteDomain
	err := c.do(ctx, http.MethodPost, "/api/v1/static-sites/"+PathEscape(name)+"/domains", map[string]string{"domain": domain}, &out)
	return out, err
}

// StaticSiteDomainStatus calls GET /api/v1/static-sites/{name}/domains/{domain}/status.
func (c *Client) StaticSiteDomainStatus(ctx context.Context, name, domain string) (StaticSiteDomain, error) {
	var out StaticSiteDomain
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/domains/"+PathEscape(strings.ToLower(domain))+"/status", nil, &out)
	return out, err
}

// StaticSiteCheck is one finding of a site health check.
type StaticSiteCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Path   string `json:"path,omitempty"`
}

// StaticSiteChecks mirrors GET /api/v1/static-sites/{name}/checks.
type StaticSiteChecks struct {
	Name         string            `json:"name"`
	Score        int               `json:"score"`
	FilesScanned int               `json:"files_scanned"`
	Truncated    bool              `json:"truncated"`
	Checks       []StaticSiteCheck `json:"checks"`
}

// StaticSiteChecks calls GET /api/v1/static-sites/{name}/checks.
func (c *Client) StaticSiteChecks(ctx context.Context, name string) (StaticSiteChecks, error) {
	var out StaticSiteChecks
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/checks", nil, &out)
	return out, err
}

// StaticSiteTraffic calls GET /api/v1/static-sites/{name}/traffic for a 24h or 7d window.
// The body is returned as a generic map so new server fields pass through untouched.
func (c *Client) StaticSiteTraffic(ctx context.Context, name, window string) (map[string]any, error) {
	var out map[string]any
	path := "/api/v1/static-sites/" + PathEscape(name) + "/traffic"
	if window != "" {
		path += "?window=" + window
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

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

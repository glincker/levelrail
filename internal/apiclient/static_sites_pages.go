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
	// Diagnosis is the status route's checklist.
	Diagnosis []StaticSiteDiagnosis `json:"diagnosis,omitempty"`
}

// StaticSiteDiagnosis is one row of a domain's status checklist.
type StaticSiteDiagnosis struct {
	Code string `json:"code"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint"`
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
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/domains/"+PathEscape(strings.ToLower(domain))+"/status?diagnose=1", nil, &out)
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

// StaticSiteGit is the optional git object of POST /api/v1/static-sites.
type StaticSiteGit struct {
	RepoURL      string `json:"repo_url"`
	Branch       string `json:"branch,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	BuildCommand string `json:"build_command,omitempty"`
	OutputDir    string `json:"output_dir,omitempty"`
	DeployOnPush *bool  `json:"deploy_on_push,omitempty"`
}

// StaticSiteCreated is the response of a create with a git source.
type StaticSiteCreated struct {
	StaticSiteDetail
	DeployID string `json:"deploy_id,omitempty"`
}

// CreateStaticSiteFromGit calls POST /api/v1/static-sites with a git source and, with deploy, starts the first build.
func (c *Client) CreateStaticSiteFromGit(ctx context.Context, name string, git StaticSiteGit, deploy bool) (StaticSiteCreated, error) {
	var out StaticSiteCreated
	err := c.do(ctx, http.MethodPost, "/api/v1/static-sites", map[string]any{"name": name, "git": git, "deploy": deploy}, &out)
	return out, err
}

// StaticSiteDeploy is one entry of GET /api/v1/static-sites/{name}/deploys.
type StaticSiteDeploy struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	URL        string `json:"url,omitempty"`
	Live       bool   `json:"live"`
	Promotable bool   `json:"promotable"`
}

// ListStaticSiteDeploys calls GET /api/v1/static-sites/{name}/deploys.
func (c *Client) ListStaticSiteDeploys(ctx context.Context, name string) ([]StaticSiteDeploy, error) {
	var out struct {
		Deploys []StaticSiteDeploy `json:"deploys"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(name)+"/deploys", nil, &out)
	return out.Deploys, err
}

// PromoteStaticSiteDeploy calls POST /api/v1/static-sites/{name}/deploys/{id}/promote.
func (c *Client) PromoteStaticSiteDeploy(ctx context.Context, name, deployID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/static-sites/"+PathEscape(name)+"/deploys/"+PathEscape(deployID)+"/promote", nil, nil)
}

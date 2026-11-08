package apiclient

import (
	"context"
	"net/http"
	"time"
)

// EdgePurge is one cache purge in an app's edge status.
type EdgePurge struct {
	At       time.Time `json:"at"`
	Trigger  string    `json:"trigger"`
	Scope    string    `json:"scope"`
	Provider string    `json:"provider"`
	Zones    []string  `json:"zones"`
	Hosts    int       `json:"hosts"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
}

// AppEdge is GET /api/v1/apps/{name}/edge.
type AppEdge struct {
	App        string   `json:"app"`
	Health     string   `json:"health"`
	Proxied    bool     `json:"proxied"`
	Override   *bool    `json:"override"`
	Provider   string   `json:"provider,omitempty"`
	Zone       string   `json:"zone,omitempty"`
	SSLMode    string   `json:"ssl_mode,omitempty"`
	SSLWarning string   `json:"ssl_warning,omitempty"`
	Domains    []string `json:"domains"`
	Deploy     *struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Source    string `json:"source"`
		CommitSHA string `json:"commit_sha,omitempty"`
	} `json:"deploy,omitempty"`
	DNS struct {
		Managed int `json:"managed_records"`
	} `json:"dns"`
	Cache struct {
		Hits    uint64  `json:"hits"`
		Misses  uint64  `json:"misses"`
		HitRate float64 `json:"hit_rate"`
		Entries int     `json:"entries"`
	} `json:"cache"`
	LastPurge *EdgePurge  `json:"last_purge,omitempty"`
	Purges    []EdgePurge `json:"purges"`
	Issues    []string    `json:"issues"`
}

// CDNSetupStep is one line of the setup checklist.
type CDNSetupStep struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// CDNSetup is the answer to POST /api/v1/apps/{name}/cdn/setup.
type CDNSetup struct {
	App      string         `json:"app"`
	Provider string         `json:"provider"`
	DryRun   bool           `json:"dry_run"`
	Applied  bool           `json:"applied"`
	Status   string         `json:"status"`
	Steps    []CDNSetupStep `json:"steps"`
}

// GetAppEdge calls GET /api/v1/apps/{name}/edge.
func (c *Client) GetAppEdge(ctx context.Context, app string) (AppEdge, error) {
	var out AppEdge
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/edge", nil, &out)
	return out, err
}

// SetupAppCDN calls POST /api/v1/apps/{name}/cdn/setup.
func (c *Client) SetupAppCDN(ctx context.Context, app, provider string, dryRun bool) (CDNSetup, error) {
	var out CDNSetup
	err := c.do(ctx, http.MethodPost, appCDNPath(app)+"/setup", struct {
		Provider string `json:"provider,omitempty"`
		DryRun   bool   `json:"dry_run"`
	}{provider, dryRun}, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// AdvisoryEntry is one advisory in the configured feed.
type AdvisoryEntry struct {
	ID        string `json:"id"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
	FixedIn   string `json:"fixed_in,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// AdvisoryMatch is one advisory that applies to one running image.
type AdvisoryMatch struct {
	App        string `json:"app"`
	Image      string `json:"image"`
	Digest     string `json:"digest,omitempty"`
	AdvisoryID string `json:"advisory_id"`
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	FixedIn    string `json:"fixed_in,omitempty"`
	Reference  string `json:"reference,omitempty"`
	Hint       string `json:"hint"`
}

// AdvisoriesResource is GET /api/v1/security/advisories.
type AdvisoriesResource struct {
	Enabled     bool            `json:"enabled"`
	Source      string          `json:"source,omitempty"`
	Signature   string          `json:"signature,omitempty"`
	Error       string          `json:"error,omitempty"`
	GeneratedAt time.Time       `json:"generated_at,omitempty"`
	LoadedAt    time.Time       `json:"loaded_at,omitempty"`
	Entries     []AdvisoryEntry `json:"entries"`
	Matches     []AdvisoryMatch `json:"matches"`
}

// GetAdvisories returns the feed state and the running images that match it.
func (c *Client) GetAdvisories(ctx context.Context) (AdvisoriesResource, error) {
	var out AdvisoriesResource
	err := c.do(ctx, http.MethodGet, "/api/v1/security/advisories", nil, &out)
	return out, err
}

// AdvisoryRefreshResult is the reply of POST /api/v1/apps/{name}/advisory-refresh.
type AdvisoryRefreshResult struct {
	Name            string `json:"name,omitempty"`
	Image           string `json:"image,omitempty"`
	DeployID        string `json:"deploy_id,omitempty"`
	PendingApproval *struct {
		ID string `json:"id"`
	} `json:"pending_approval,omitempty"`
}

// RefreshAdvisoryApp redeploys an app on the newest digest of its tag.
func (c *Client) RefreshAdvisoryApp(ctx context.Context, app string, confirm bool) (AdvisoryRefreshResult, error) {
	var out AdvisoryRefreshResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+url.PathEscape(app)+"/advisory-refresh", map[string]bool{"confirm": confirm}, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// GitRepoResource mirrors internal/repolist.Repo: one repository in
// GET /api/v1/git/repos.
type GitRepoResource struct {
	Key           string `json:"key"`
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Owner         string `json:"owner"`
	Private       bool   `json:"private"`
	Archived      bool   `json:"archived"`
	Fork          bool   `json:"fork"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	WebURL        string `json:"web_url,omitempty"`
	Language      string `json:"language,omitempty"`
	PushedAt      string `json:"pushed_at,omitempty"`
	ProjectID     int64  `json:"project_id,omitempty"`
	AccountType   string `json:"account_type,omitempty"`
}

// GitRepoOwnerCount is one owner facet of a repository listing.
type GitRepoOwnerCount struct {
	Login string `json:"login"`
	Count int    `json:"count"`
}

// GitRepoRateLimit says when a rate limited provider may be asked again.
type GitRepoRateLimit struct {
	NextCheckAt       time.Time `json:"next_check_at"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
}

// GitRepoListResource mirrors internal/api's gitRepoListResource.
type GitRepoListResource struct {
	Provider   string                         `json:"provider"`
	Items      []GitRepoResource              `json:"items"`
	Total      int                            `json:"total"`
	Loaded     int                            `json:"loaded"`
	Limit      int                            `json:"limit"`
	Offset     int                            `json:"offset"`
	NextOffset *int                           `json:"next_offset"`
	HasMore    bool                           `json:"has_more"`
	Truncated  bool                           `json:"truncated"`
	Cap        int                            `json:"cap"`
	CachedAt   time.Time                      `json:"cached_at"`
	Stale      bool                           `json:"stale"`
	Refreshing bool                           `json:"refreshing"`
	Owners     []GitRepoOwnerCount            `json:"owners"`
	Errors     []GitHubAppRepoListErrResource `json:"errors,omitempty"`
	RateLimit  *GitRepoRateLimit              `json:"rate_limit,omitempty"`
}

// GitRepoListParams narrows and pages ListGitRepos. Archived and Forks take
// include, exclude or only; empty means include.
type GitRepoListParams struct {
	Provider string
	Search   string
	Owner    string
	Archived string
	Forks    string
	Limit    int
	Offset   int
	Refresh  bool
}

// ListGitRepos calls GET /api/v1/git/repos: one provider's repositories,
// searched, filtered and paged on the server from its cached list.
func (c *Client) ListGitRepos(ctx context.Context, p GitRepoListParams) (GitRepoListResource, error) {
	q := url.Values{"provider": {p.Provider}}
	for k, v := range map[string]string{"search": p.Search, "owner": p.Owner, "archived": p.Archived, "forks": p.Forks} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if p.Refresh {
		q.Set("refresh", "1")
	}
	var out GitRepoListResource
	err := c.do(ctx, http.MethodGet, "/api/v1/git/repos?"+q.Encode(), nil, &out)
	return out, err
}

// GitWebhookRegistrationResource mirrors internal/api's gitWebhookRegistrationResource.
type GitWebhookRegistrationResource struct {
	State     string     `json:"state"`
	Error     string     `json:"error,omitempty"`
	ChangedAt *time.Time `json:"changed_at,omitempty"`
}

// GitPollStatusResource mirrors internal/api's gitPollStatusResource.
type GitPollStatusResource struct {
	Enabled       bool       `json:"enabled"`
	LastPolledAt  *time.Time `json:"last_polled_at,omitempty"`
	LastWebhookAt *time.Time `json:"last_webhook_at,omitempty"`
	LastSeenHead  string     `json:"last_seen_head,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// RegisterGitSourceWebhook calls POST /api/v1/apps/{name}/git-source/webhook/register:
// registers the push webhook on the provider again and returns the source with the new outcome.
func (c *Client) RegisterGitSourceWebhook(ctx context.Context, name string) (GitSourceResource, error) {
	var out GitSourceResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/git-source/webhook/register", nil, &out)
	return out, err
}

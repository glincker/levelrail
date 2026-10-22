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
	// Provider, ConnectionID, ConnectionLabel and Account say which account the row came from.
	Provider        string `json:"provider,omitempty"`
	ConnectionID    string `json:"connection_id,omitempty"`
	ConnectionLabel string `json:"connection_label,omitempty"`
	Account         string `json:"account,omitempty"`
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
	// Connections lists every account the merged list was built from.
	Connections []GitRepoConnectionFacet `json:"connections"`
}

// GitRepoConnectionFacet is one account behind a repository listing.
type GitRepoConnectionFacet struct {
	ConnectionID string `json:"connection_id"`
	Provider     string `json:"provider"`
	Label        string `json:"label"`
	Account      string `json:"account,omitempty"`
	Loaded       int    `json:"loaded"`
	Failed       bool   `json:"failed"`
}

// GitRepoListParams narrows and pages ListGitRepos. Archived and Forks take
// include, exclude or only; empty means include.
type GitRepoListParams struct {
	// Provider is optional: empty merges every connected account of every provider.
	Provider string
	// Connection narrows the list to one connection id.
	Connection string
	Search     string
	Owner      string
	Archived   string
	Forks      string
	Limit      int
	Offset     int
	Refresh    bool
}

// ListGitRepos calls GET /api/v1/git/repos: one provider's repositories,
// searched, filtered and paged on the server from its cached list.
func (c *Client) ListGitRepos(ctx context.Context, p GitRepoListParams) (GitRepoListResource, error) {
	q := url.Values{}
	for k, v := range map[string]string{"provider": p.Provider, "connection": p.Connection, "search": p.Search, "owner": p.Owner, "archived": p.Archived, "forks": p.Forks} {
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

// GitProviderConnectionResource mirrors internal/api's gitConnectionResource:
// one GitLab, Bitbucket or Gitea account. It carries no credential.
type GitProviderConnectionResource struct {
	ID            string                `json:"id"`
	Provider      string                `json:"provider"`
	Label         string                `json:"label"`
	Host          string                `json:"host"`
	InstanceURL   string                `json:"instance_url"`
	ClientID      string                `json:"client_id"`
	AccountID     string                `json:"account_id,omitempty"`
	AccountName   string                `json:"account_name,omitempty"`
	Scopes        string                `json:"scopes,omitempty"`
	IsDefault     bool                  `json:"is_default"`
	Authorized    bool                  `json:"authorized"`
	CreatedBy     string                `json:"created_by,omitempty"`
	CreatedAt     string                `json:"created_at"`
	SourceCount   int                   `json:"source_count"`
	Connection    GitConnectionResource `json:"connection"`
	AuthorizePath string                `json:"authorize_path"`
	AuthMethod    string                `json:"auth_method,omitempty"`
}

// CreateGitConnectionRequest registers an OAuth application as a new connection.
type CreateGitConnectionRequest struct {
	Provider          string `json:"provider"`
	InstanceURL       string `json:"instance_url,omitempty"`
	ClientID          string `json:"client_id,omitempty"`
	ClientSecret      string `json:"client_secret,omitempty"`
	Label             string `json:"label,omitempty"`
	ReuseConnectionID string `json:"reuse_connection_id,omitempty"`
	// Token is a personal access token, used instead of the OAuth fields (any host).
	Token string `json:"token,omitempty"`
}

// GitDisconnectResource mirrors internal/api's gitDisconnectResource.
type GitDisconnectResource struct {
	Disconnected    bool     `json:"disconnected"`
	TokenRevoked    bool     `json:"token_revoked"`
	Note            string   `json:"note"`
	AffectedSources []string `json:"affected_sources,omitempty"`
}

// ListGitConnections calls GET /api/v1/git-connections; provider may be empty.
func (c *Client) ListGitConnections(ctx context.Context, provider string) ([]GitProviderConnectionResource, error) {
	path := "/api/v1/git-connections"
	if provider != "" {
		path += "?provider=" + url.QueryEscape(provider)
	}
	var out []GitProviderConnectionResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// CreateGitConnection calls POST /api/v1/git-connections.
func (c *Client) CreateGitConnection(ctx context.Context, req CreateGitConnectionRequest) (GitProviderConnectionResource, error) {
	var out GitProviderConnectionResource
	err := c.do(ctx, http.MethodPost, "/api/v1/git-connections", req, &out)
	return out, err
}

// TestGitConnectionRequest checks an instance URL and optional token without saving a connection.
type TestGitConnectionRequest struct {
	Provider    string `json:"provider"`
	InstanceURL string `json:"instance_url,omitempty"`
	Token       string `json:"token,omitempty"`
}

// TestGitConnectionResult is the typed outcome: Kind is ok, bad_url, bad_token, tls or unreachable.
type TestGitConnectionResult struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
	Account string `json:"account,omitempty"`
	// Token is set when a token was checked: its scopes and how many repositories it reaches.
	Token *GitTokenDetails `json:"token,omitempty"`
}

// GitTokenDetails mirrors internal/api's gitTokenDetails.
type GitTokenDetails struct {
	Scopes       []string `json:"scopes,omitempty"`
	ScopesKnown  bool     `json:"scopes_known"`
	Missing      []string `json:"missing_scopes,omitempty"`
	RepoAccess   bool     `json:"repo_access"`
	RepoCount    int      `json:"repo_count"`
	RepoCountCap bool     `json:"repo_count_capped,omitempty"`
}

// TestGitConnection calls POST /api/v1/git-connections/test.
func (c *Client) TestGitConnection(ctx context.Context, req TestGitConnectionRequest) (TestGitConnectionResult, error) {
	var out TestGitConnectionResult
	err := c.do(ctx, http.MethodPost, "/api/v1/git-connections/test", req, &out)
	return out, err
}

// RenameGitConnection calls PATCH /api/v1/git-connections/{id}.
func (c *Client) RenameGitConnection(ctx context.Context, id, label string) (GitProviderConnectionResource, error) {
	var out GitProviderConnectionResource
	err := c.do(ctx, http.MethodPatch, "/api/v1/git-connections/"+PathEscape(id), map[string]string{"label": label}, &out)
	return out, err
}

// SetDefaultGitConnection calls POST /api/v1/git-connections/{id}/default.
func (c *Client) SetDefaultGitConnection(ctx context.Context, id string) (GitProviderConnectionResource, error) {
	var out GitProviderConnectionResource
	err := c.do(ctx, http.MethodPost, "/api/v1/git-connections/"+PathEscape(id)+"/default", nil, &out)
	return out, err
}

// DisconnectGitConnection calls DELETE /api/v1/git-connections/{id}.
func (c *Client) DisconnectGitConnection(ctx context.Context, id string) (GitDisconnectResource, error) {
	var out GitDisconnectResource
	err := c.do(ctx, http.MethodDelete, "/api/v1/git-connections/"+PathEscape(id), nil, &out)
	return out, err
}

// ListGitConnectionBranches calls GET /api/v1/git-connections/{id}/branches?repo=.
func (c *Client) ListGitConnectionBranches(ctx context.Context, id, repo string) ([]GitAppBranchResource, error) {
	var out []GitAppBranchResource
	err := c.do(ctx, http.MethodGet, "/api/v1/git-connections/"+PathEscape(id)+"/branches?repo="+url.QueryEscape(repo), nil, &out)
	return out, err
}

// UseGitConnectionRepoAsSource calls POST /api/v1/git-connections/{id}/use-as-source.
func (c *Client) UseGitConnectionRepoAsSource(ctx context.Context, id string, req UseRepoAsSourceRequest) (GitSourceResource, error) {
	var out GitSourceResource
	err := c.do(ctx, http.MethodPost, "/api/v1/git-connections/"+PathEscape(id)+"/use-as-source", req, &out)
	return out, err
}

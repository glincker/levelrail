// Package giteaapp implements OAuth2 and the small slice of Gitea's
// REST API this control plane needs once connected.
//
// Methods take instanceURL explicitly rather than baking one into the
// Client, the same reasoning internal/gitlabapp's own doc comment gives:
// Gitea is almost always self-hosted, never a fixed host like
// api.github.com. Repos are addressed by an "owner/repo" pair, not a
// numeric ID: Gitea's REST API is GitHub-shaped here, unlike GitLab's
// own numeric project ID.
package giteaapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// Client is a small, purpose-built Gitea REST/OAuth client, not a
// general-purpose Gitea SDK.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with a bounded per-request timeout, the
// same reasoning internal/githubapp.NewClient's own doc comment gives.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// apiError is giteaapp's own name for the shared gitprovider.APIError,
// kept as a distinct type so callers and tests can refer to it without
// importing internal/gitprovider directly.
type apiError = gitprovider.APIError

const (
	errPrefix = "giteaapp"
	apiName   = "gitea"
)

func apiBaseURL(instanceURL string) string {
	return strings.TrimRight(instanceURL, "/") + "/api/v1"
}

func (c *Client) do(ctx context.Context, method, fullURL, authHeader string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", errPrefix, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return gitprovider.Execute(c.HTTP, req, errPrefix, apiName, method+" "+fullURL, out)
}

// Tokens is ExchangeCode's and RefreshToken's shared result.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// expiryMargin mirrors internal/gitlabapp's own constant and reasoning:
// subtracted from a freshly minted token's reported expiry so a caller
// checking "is this still valid" moments before use doesn't lose a race
// against the token expiring mid-request.
const expiryMargin = 30 * time.Second

func toTokens(resp tokenResponse) Tokens {
	var expiresAt time.Time
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Add(-expiryMargin)
	}
	return Tokens{AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken, ExpiresAt: expiresAt}
}

// ExchangeCode exchanges an OAuth authorization code for an access
// token, Gitea's documented POST {instance}/login/oauth/access_token
// (docs.gitea.com/development/oauth2-provider).
func (c *Client) ExchangeCode(ctx context.Context, instanceURL, clientID, clientSecret, redirectURI, code string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	return c.token(ctx, instanceURL, form)
}

// RefreshToken exchanges a still-valid refresh token for a new access
// token, Gitea's grant_type "refresh_token" on the same endpoint
// ExchangeCode uses.
func (c *Client) RefreshToken(ctx context.Context, instanceURL, clientID, clientSecret, refreshToken string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"refresh_token": {refreshToken},
	}
	return c.token(ctx, instanceURL, form)
}

func (c *Client) token(ctx context.Context, instanceURL string, form url.Values) (Tokens, error) {
	tokenURL := strings.TrimRight(instanceURL, "/") + "/login/oauth/access_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("%s: build token request: %w", errPrefix, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var out tokenResponse
	if err := gitprovider.Execute(c.HTTP, req, errPrefix, apiName, http.MethodPost+" "+tokenURL, &out); err != nil {
		return Tokens{}, err
	}
	return toTokens(out), nil
}

// Repo is one Gitea repository accessible to the connected account, the
// subset of Gitea's Repository object this control plane's repo picker
// and "use as source" action need.
type Repo struct {
	FullName      string
	Name          string
	Private       bool
	DefaultBranch string
	CloneURL      string
	WebURL        string
}

type repoResponse struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	HTMLURL       string `json:"html_url"`
}

func toRepo(r repoResponse) Repo {
	return Repo{
		FullName:      r.FullName,
		Name:          r.Name,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		CloneURL:      r.CloneURL,
		WebURL:        r.HTMLURL,
	}
}

// listPerPage and listPageCap mirror internal/gitlabapp.Client's own
// pagination constants and reasoning exactly: Gitea pages by page number
// like GitLab, not a cursor URL like Bitbucket.
const (
	listPerPage = 50
	listPageCap = 20
)

// ListRepos lists every repository the token's account owns or is a
// collaborator on, across as many pages as Gitea returns full pages for.
func (c *Client) ListRepos(ctx context.Context, instanceURL, accessToken string) ([]Repo, error) {
	var out []Repo
	for page := 1; page <= listPageCap; page++ {
		var resp []repoResponse
		u := fmt.Sprintf("%s/user/repos?limit=%d&page=%d", apiBaseURL(instanceURL), listPerPage, page)
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp {
			out = append(out, toRepo(r))
		}
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

// GetRepo looks up a single repository by its "owner/repo" full name.
func (c *Client) GetRepo(ctx context.Context, instanceURL, accessToken, fullName string) (Repo, error) {
	var resp repoResponse
	u := apiBaseURL(instanceURL) + "/repos/" + fullName
	if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
		return Repo{}, err
	}
	return toRepo(resp), nil
}

// Branch is one branch of one repository.
type Branch struct {
	Name      string
	CommitSHA string
}

type branchResponse struct {
	Name   string `json:"name"`
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
}

// ListBranches lists every branch of fullName ("owner/repo"), across as
// many pages as Gitea returns full pages for, the same pagination shape
// ListRepos uses.
func (c *Client) ListBranches(ctx context.Context, instanceURL, accessToken, fullName string) ([]Branch, error) {
	var out []Branch
	for page := 1; page <= listPageCap; page++ {
		var resp []branchResponse
		u := fmt.Sprintf("%s/repos/%s/branches?limit=%d&page=%d", apiBaseURL(instanceURL), fullName, listPerPage, page)
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp {
			out = append(out, Branch{Name: b.Name, CommitSHA: b.Commit.ID})
		}
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

type giteaWebhookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret"`
}

type createWebhookRequest struct {
	Type   string             `json:"type"`
	Config giteaWebhookConfig `json:"config"`
	Events []string           `json:"events"`
	Active bool               `json:"active"`
}

// CreateRepoWebhook registers a push and pull_request webhook on
// fullName ("owner/repo") pointed at hookURL, with secret set as the
// value Gitea signs each delivery's X-Gitea-Signature header with
// (HMAC-SHA256, hex digest, no prefix). Gitea also sends
// X-Hub-Signature-256 in GitHub's own "sha256=<hex>" format for
// compatibility (docs.gitea.com/usage/webhooks), which is the header
// verifyGitPushWebhookAuth (internal/api/git_webhook.go) actually
// checks, so this package needs no signature-parsing code of its own.
// pull_request must be requested explicitly: without it Gitea never
// delivers the event internal/webhook's Gitea pull-request parsing
// expects, so preview environments would silently never trigger for a
// connected repo, the same reasoning GitLab's and Bitbucket's own
// CreateProjectWebhook/CreateRepoWebhook already give for their own
// merge/pull-request event keys.
func (c *Client) CreateRepoWebhook(ctx context.Context, instanceURL, accessToken, fullName, hookURL, secret string) error {
	body, err := json.Marshal(createWebhookRequest{ //nolint:gosec // secret is sent to Gitea to configure delivery signing, not a leaked credential
		Type:   "gitea",
		Config: giteaWebhookConfig{URL: hookURL, ContentType: "json", Secret: secret},
		Events: []string{"push", "pull_request"},
		Active: true,
	})
	if err != nil {
		return fmt.Errorf("giteaapp: marshal webhook request: %w", err)
	}
	u := apiBaseURL(instanceURL) + "/repos/" + fullName + "/hooks"
	return c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(body), nil)
}

type createIssueCommentRequest struct {
	Body string `json:"body"`
}

// CommitStatusState is Gitea's own documented "state" enum for
// POST .../statuses/{sha}.
type CommitStatusState string

const (
	// CommitStatusPending marks a commit status as still in progress.
	CommitStatusPending CommitStatusState = "pending"
	// CommitStatusSuccess marks a commit status as succeeded.
	CommitStatusSuccess CommitStatusState = "success"
	// CommitStatusFailure marks a commit status as failed.
	CommitStatusFailure CommitStatusState = "failure"
)

type createCommitStatusRequest struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
}

// CreateCommitStatus sets a commit status on sha of fullName
// ("owner/repo"), authenticated the same way CreateIssueComment is.
func (c *Client) CreateCommitStatus(ctx context.Context, instanceURL, accessToken, fullName, sha string, state CommitStatusState, targetURL, description, statusContext string) error {
	payload, err := json.Marshal(createCommitStatusRequest{
		State: string(state), TargetURL: targetURL, Description: description, Context: statusContext,
	})
	if err != nil {
		return fmt.Errorf("giteaapp: marshal commit status request: %w", err)
	}
	u := fmt.Sprintf("%s/repos/%s/statuses/%s", apiBaseURL(instanceURL), fullName, url.PathEscape(sha))
	return c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(payload), nil)
}

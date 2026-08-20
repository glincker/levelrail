// Package bitbucketapp implements OAuth2 and the small slice of
// Bitbucket Cloud's REST API this control plane needs once connected.
//
// Bitbucket Cloud only: no instance URL parameter anywhere in this
// package, unlike internal/gitlabapp. Bitbucket Server/Data Center has
// no OAuth-consumer equivalent and is out of scope
// (docs/design/git-provider-integrations.md section 3); a Data Center
// repo can still be connected today through the existing generic
// deploy-token path (PUT /api/v1/apps/{name}/git-source).
package bitbucketapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultOAuthTokenURL = "https://bitbucket.org/site/oauth2/access_token" //nolint:gosec // a public endpoint URL, not a credential
	defaultAPIBaseURL    = "https://api.bitbucket.org/2.0"
)

// Client is a small, purpose-built Bitbucket Cloud REST/OAuth client,
// not a general-purpose Bitbucket SDK. TokenURL/APIBaseURL are
// overridable, the same "tests point this at an httptest.Server"
// pattern internal/githubapp.Client's own BaseURL field establishes:
// production code always leaves them empty and gets the real Bitbucket
// Cloud hosts.
type Client struct {
	HTTP       *http.Client
	TokenURL   string
	APIBaseURL string
}

// NewClient returns a Client with a bounded per-request timeout, the
// same reasoning internal/githubapp.NewClient's own doc comment gives.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) tokenURL() string {
	if c.TokenURL != "" {
		return c.TokenURL
	}
	return defaultOAuthTokenURL
}

func (c *Client) apiBaseURL() string {
	if c.APIBaseURL != "" {
		return c.APIBaseURL
	}
	return defaultAPIBaseURL
}

type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("bitbucketapp: bitbucket api returned %d: %s", e.StatusCode, e.Body)
}

const maxErrorBodySnippet = 512

func (c *Client) do(ctx context.Context, method, fullURL, authHeader string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("bitbucketapp: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucketapp: request %s %s: %w", method, fullURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySnippet))
		return &apiError{StatusCode: resp.StatusCode, Body: string(snippet)}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("bitbucketapp: decode response for %s %s: %w", method, fullURL, err)
	}
	return nil
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
// against the token expiring mid-request. Bitbucket's own access tokens
// live only 2 hours (docs/design/git-provider-integrations.md section
// 3), materially shorter than GitHub's ~1h installation tokens or
// GitLab's own, so refresh here is the normal path, not an edge case.
const expiryMargin = 30 * time.Second

func toTokens(resp tokenResponse) Tokens {
	var expiresAt time.Time
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Add(-expiryMargin)
	}
	return Tokens{AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken, ExpiresAt: expiresAt}
}

// basicAuth builds the Authorization header Bitbucket's own OAuth2 token
// endpoint requires: HTTP Basic with the OAuth consumer's key and secret,
// not a form field the way GitLab's ExchangeCode sends client_id/
// client_secret (https://developer.atlassian.com/cloud/bitbucket/oauth-2/).
func basicAuth(key, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(key+":"+secret))
}

// ExchangeCode exchanges an OAuth authorization code for an access
// token.
func (c *Client) ExchangeCode(ctx context.Context, key, secret, code string) (Tokens, error) {
	form := url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
	}
	return c.token(ctx, key, secret, form)
}

// RefreshToken exchanges a still-valid refresh token for a new access
// token.
func (c *Client) RefreshToken(ctx context.Context, key, secret, refreshToken string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return c.token(ctx, key, secret, form)
}

func (c *Client) token(ctx context.Context, key, secret string, form url.Values) (Tokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("bitbucketapp: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", basicAuth(key, secret))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("bitbucketapp: token request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySnippet))
		return Tokens{}, &apiError{StatusCode: resp.StatusCode, Body: string(snippet)}
	}
	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Tokens{}, fmt.Errorf("bitbucketapp: decode token response: %w", err)
	}
	return toTokens(out), nil
}

// Repo is one Bitbucket repository accessible to the connected account,
// the subset of Bitbucket's Repository object this control plane's repo
// picker and "use as source" action need. FullName ("workspace/repo_slug")
// is the identifier callers pass back to GetRepo/ListBranches/
// CreateWebhook: Bitbucket has no separate numeric ID in the sense
// GitLab's Project.ID is used here.
type Repo struct {
	FullName      string
	Name          string
	Private       bool
	DefaultBranch string
	CloneURL      string
	WebURL        string
}

type repoLinkHref struct {
	Href string `json:"href"`
}

type repoResponse struct {
	FullName   string `json:"full_name"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"is_private"`
	MainBranch struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
	Links struct {
		HTML  repoLinkHref `json:"html"`
		Clone []cloneLink  `json:"clone"`
	} `json:"links"`
}

type cloneLink struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

func toRepo(r repoResponse) Repo {
	repo := Repo{
		FullName:      r.FullName,
		Name:          r.Name,
		Private:       r.IsPrivate,
		DefaultBranch: r.MainBranch.Name,
		WebURL:        r.Links.HTML.Href,
	}
	for _, link := range r.Links.Clone {
		if link.Name == "https" {
			repo.CloneURL = link.Href
			break
		}
	}
	if repo.CloneURL == "" {
		repo.CloneURL = "https://bitbucket.org/" + r.FullName + ".git"
	}
	return repo
}

// listPageCap bounds how many pages ListRepos/ListBranches will ever
// follow, the same safety-valve reasoning internal/githubapp.Client's
// own listPageCap gives, sized for Bitbucket's smaller default page
// size (10, vs GitHub/GitLab's 100): 20 pages * up to 100 requested
// per_page covers the same realistic ceiling.
const listPageCap = 20

type pagedRepoResponse struct {
	Values []repoResponse `json:"values"`
	Next   string         `json:"next"`
}

// ListRepos lists every repository the token's account is at least a
// member of. Bitbucket paginates by an opaque "next" URL in the
// response body, not a page number the caller increments (GitHub's and
// GitLab's own shape): each iteration below follows that URL directly
// rather than constructing one, until the response omits it or
// listPageCap is hit.
func (c *Client) ListRepos(ctx context.Context, accessToken string) ([]Repo, error) {
	var out []Repo
	next := c.apiBaseURL() + "/repositories?role=member&pagelen=100"
	for page := 0; page < listPageCap && next != ""; page++ {
		var resp pagedRepoResponse
		if err := c.do(ctx, http.MethodGet, next, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Values {
			out = append(out, toRepo(r))
		}
		next = resp.Next
	}
	return out, nil
}

// GetRepo looks up a single repository by its "workspace/repo_slug"
// full name.
func (c *Client) GetRepo(ctx context.Context, accessToken, fullName string) (Repo, error) {
	var resp repoResponse
	u := c.apiBaseURL() + "/repositories/" + fullName
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
	Target struct {
		Hash string `json:"hash"`
	} `json:"target"`
}

type pagedBranchResponse struct {
	Values []branchResponse `json:"values"`
	Next   string           `json:"next"`
}

// ListBranches lists every branch of fullName ("workspace/repo_slug"),
// the same cursor-pagination shape ListRepos follows.
func (c *Client) ListBranches(ctx context.Context, accessToken, fullName string) ([]Branch, error) {
	var out []Branch
	next := c.apiBaseURL() + "/repositories/" + fullName + "/refs/branches?pagelen=100"
	for page := 0; page < listPageCap && next != ""; page++ {
		var resp pagedBranchResponse
		if err := c.do(ctx, http.MethodGet, next, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp.Values {
			out = append(out, Branch{Name: b.Name, CommitSHA: b.Target.Hash})
		}
		next = resp.Next
	}
	return out, nil
}

type createWebhookRequest struct {
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Active      bool     `json:"active"`
	Events      []string `json:"events"`
	Secret      string   `json:"secret"`
}

// CreateRepoWebhook registers a repo:push webhook on fullName
// ("workspace/repo_slug") pointed at hookURL, with secret set as the
// value Bitbucket signs each delivery's X-Hub-Signature header with
// (HMAC-SHA256, the same header name GitHub uses for its own SHA-1
// predecessor, see internal/api/git_webhook.go's own verification
// branch for the distinction).
func (c *Client) CreateRepoWebhook(ctx context.Context, accessToken, fullName, hookURL, secret string) error {
	body, err := json.Marshal(createWebhookRequest{ //nolint:gosec // secret is sent to Bitbucket to configure delivery signing, not a leaked credential
		Description: "Levelrail push deploy",
		URL:         hookURL,
		Active:      true,
		Events:      []string{"repo:push"},
		Secret:      secret,
	})
	if err != nil {
		return fmt.Errorf("bitbucketapp: marshal webhook request: %w", err)
	}
	u := c.apiBaseURL() + "/repositories/" + fullName + "/hooks"
	return c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(body), nil)
}

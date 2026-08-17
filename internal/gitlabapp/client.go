// Package gitlabapp implements OAuth2 and the small slice of GitLab's
// REST API this control plane needs once connected.
//
// Methods take instanceURL explicitly rather than baking one into the
// Client, since GitLab is commonly self-hosted and not fixed like
// api.github.com.
package gitlabapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a small, purpose-built GitLab REST/OAuth client, not a
// general-purpose GitLab SDK.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with a bounded per-request timeout, the
// same reasoning internal/githubapp.NewClient's own doc comment gives.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("gitlabapp: gitlab api returned %d: %s", e.StatusCode, e.Body)
}

const maxErrorBodySnippet = 512

func apiBaseURL(instanceURL string) string {
	return strings.TrimRight(instanceURL, "/") + "/api/v4"
}

func (c *Client) do(ctx context.Context, method, fullURL, authHeader string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("gitlabapp: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("gitlabapp: request %s %s: %w", method, fullURL, err)
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
		return fmt.Errorf("gitlabapp: decode response for %s %s: %w", method, fullURL, err)
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
	CreatedAt    int64  `json:"created_at"`
}

// expiryMargin is subtracted from a freshly minted token's own reported
// expiry, so a caller that checks "is this still valid" a few seconds
// before actually using it doesn't lose a race against the token
// expiring mid-request.
const expiryMargin = 30 * time.Second

func toTokens(resp tokenResponse) Tokens {
	var expiresAt time.Time
	if resp.ExpiresIn > 0 {
		created := time.Unix(resp.CreatedAt, 0)
		if resp.CreatedAt == 0 {
			created = time.Now()
		}
		expiresAt = created.Add(time.Duration(resp.ExpiresIn) * time.Second).Add(-expiryMargin)
	}
	return Tokens{AccessToken: resp.AccessToken, RefreshToken: resp.RefreshToken, ExpiresAt: expiresAt}
}

// ExchangeCode exchanges an OAuth authorization code for an access
// token, GitLab's documented POST {instance}/oauth/token grant_type
// "authorization_code" (docs.gitlab.com/ee/api/oauth2.html).
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
// token, GitLab's grant_type "refresh_token" on the same endpoint
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(instanceURL, "/")+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("gitlabapp: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("gitlabapp: token request: %w", err)
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
		return Tokens{}, fmt.Errorf("gitlabapp: decode token response: %w", err)
	}
	return toTokens(out), nil
}

// Project is one GitLab project accessible to the connected account, the
// subset of GitLab's Project object this control plane's project picker
// and "use as source" action need.
type Project struct {
	ID                int64
	Name              string
	PathWithNamespace string
	HTTPURLToRepo     string
	DefaultBranch     string
	Visibility        string
	WebURL            string
}

type projectResponse struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
}

func toProject(r projectResponse) Project {
	return Project(r)
}

// listPerPage and listPageCap mirror internal/githubapp.Client's own
// pagination constants and reasoning exactly.
const (
	listPerPage = 100
	listPageCap = 20
)

// ListProjects lists every project the token's account is at least a
// member of, across as many pages as GitLab returns full pages for.
func (c *Client) ListProjects(ctx context.Context, instanceURL, accessToken string) ([]Project, error) {
	var out []Project
	for page := 1; page <= listPageCap; page++ {
		var resp []projectResponse
		u := fmt.Sprintf("%s/projects?membership=true&simple=true&order_by=last_activity_at&per_page=%d&page=%d",
			apiBaseURL(instanceURL), listPerPage, page)
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, p := range resp {
			out = append(out, toProject(p))
		}
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

// GetProject looks up a single project by ID.
func (c *Client) GetProject(ctx context.Context, instanceURL, accessToken string, projectID int64) (Project, error) {
	var resp projectResponse
	u := apiBaseURL(instanceURL) + "/projects/" + strconv.FormatInt(projectID, 10)
	if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
		return Project{}, err
	}
	return toProject(resp), nil
}

type createWebhookRequest struct {
	URL                   string `json:"url"`
	PushEvents            bool   `json:"push_events"`
	Token                 string `json:"token"`
	EnableSSLVerification bool   `json:"enable_ssl_verification"`
}

// CreateProjectWebhook registers a push-events webhook on projectID
// pointed at hookURL, authenticated by GitLab's own X-Gitlab-Token
// scheme (secretToken echoed back on every delivery, not an HMAC
// signature the way GitHub's webhooks work).
func (c *Client) CreateProjectWebhook(ctx context.Context, instanceURL, accessToken string, projectID int64, hookURL, secretToken string) error {
	body, err := json.Marshal(createWebhookRequest{
		URL: hookURL, PushEvents: true, Token: secretToken, EnableSSLVerification: true,
	})
	if err != nil {
		return fmt.Errorf("gitlabapp: marshal webhook request: %w", err)
	}
	u := apiBaseURL(instanceURL) + "/projects/" + strconv.FormatInt(projectID, 10) + "/hooks"
	return c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(body), nil)
}

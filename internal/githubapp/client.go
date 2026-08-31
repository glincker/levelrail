package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// defaultBaseURL is api.github.com's REST API root. Overridable
// (Client.BaseURL) so tests can point this at an httptest.Server
// instead of making real network calls.
const defaultBaseURL = "https://api.github.com"

// defaultInstanceURL is github.com itself: every GitHubAppConnection
// row's InstanceURL defaults to this (migrations/0061), and it is the
// one instance value APIBaseURL treats as "use defaultBaseURL /
// Client.BaseURL" rather than deriving a REST root from the instance
// URL itself.
const defaultInstanceURL = "https://github.com"

// APIBaseURL derives the REST API root for instanceURL: github.com
// itself routes through the dedicated api.github.com host
// (defaultBaseURL, or Client.BaseURL when a test has overridden it), but
// a GitHub Enterprise Server instance has no such split host, its REST
// API lives under its own domain at /api/v3
// (https://docs.github.com/en/enterprise-server/rest/about-the-rest-api).
// instanceURL is treated as already-normalized (no trailing slash, a
// real https URL): callers validate that once, at connect time, so this
// pure derivation doesn't have to.
func (c *Client) APIBaseURL(instanceURL string) string {
	if instanceURL == "" || instanceURL == defaultInstanceURL {
		return c.baseURL()
	}
	return instanceURL + "/api/v3"
}

// CheckInstanceReachable calls the unauthenticated GET /meta endpoint
// (present on both api.github.com and a GHES instance's own /api/v3),
// turning a GitHub Enterprise Server base URL an operator mistyped, or
// one this control plane simply can't route to, into an immediate,
// actionable save-time error instead of a silent failure the first time
// the manifest flow's redirect actually needs it. Not called for
// github.com itself (APIBaseURL's own default case): that host's
// reachability is not this control plane's to diagnose.
func (c *Client) CheckInstanceReachable(ctx context.Context, instanceURL string) error {
	return c.do(ctx, c.APIBaseURL(instanceURL), http.MethodGet, "/meta", "", nil)
}

// apiVersion is the value GitHub's REST API docs currently document for
// the X-GitHub-Api-Version header
// (https://docs.github.com/en/rest/about-the-rest-api/api-versions):
// 2022-11-28, the version GitHub introduced when they added this header
// requirement and have kept current since. Sent on every request this
// client makes, per GitHub's own recommendation to always pin an
// explicit version rather than floating on whatever the default is.
const apiVersion = "2022-11-28"

const bearerPrefix = "Bearer "

// Client is a small, purpose-built GitHub REST API client covering only
// what this control plane needs: manifest code exchange, installation
// lookup, installation token minting, and repository/branch listing.
// Not a general-purpose GitHub SDK.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// NewClient returns a Client pointed at the real api.github.com, with a
// bounded per-request timeout: every call this client makes is a single
// small JSON request/response, not a long-lived stream, so a generous
// fixed timeout (rather than relying solely on ctx) guards against a
// hung connection blocking an HTTP handler indefinitely.
func NewClient() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		BaseURL: defaultBaseURL,
	}
}

// apiError is returned for any non-2xx GitHub API response. Carries the
// status code (callers, e.g. internal/api's installation lookup, branch
// on this to distinguish "not found" from "server error") and a
// truncated body snippet for logs. GitHub error response bodies are
// ordinary JSON describing what went wrong with the request, never an
// echo of request credentials, so including a snippet here is safe;
// callers still only log the wrapped error's Error() string, never a
// full raw body.
type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("githubapp: github api returned %d: %s", e.StatusCode, e.Body)
}

// maxErrorBodySnippet caps how much of an error response body apiError
// retains, so a misbehaving upstream can't inflate a log line
// unboundedly.
const maxErrorBodySnippet = 512

// do issues a request against every endpoint this client calls: every
// one of them is a GET or a POST with no JSON request body (path/query
// parameters and the Authorization header carry everything each call
// needs), so this deliberately takes no body parameter rather than
// carrying one every current call site would pass as nil; a future
// endpoint that does need one is a small, explicit addition here, not a
// speculative parameter kept for its own sake.
func (c *Client) do(ctx context.Context, baseURL, method, path string, authHeader string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("githubapp: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("githubapp: request %s %s: %w", method, path, err)
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
		return fmt.Errorf("githubapp: decode response for %s %s: %w", method, path, err)
	}
	return nil
}

// doWithBody is do's sibling for the one call in this client that sends
// a JSON request body (CreateRepoWebhook): every other endpoint is a
// GET or a bodyless POST, which is why do itself deliberately has no
// body parameter (see its own doc comment).
func (c *Client) doWithBody(ctx context.Context, baseURL, method, path, authHeader string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("githubapp: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("githubapp: request %s %s: %w", method, path, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySnippet))
		apiErr := &apiError{StatusCode: resp.StatusCode, Body: string(snippet)}
		if resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%w: %w", ErrPermissionDenied, apiErr)
		}
		return apiErr
	}
	return nil
}

// ErrPermissionDenied wraps the API error GitHub returns for a request
// the installation's granted permissions don't allow, e.g.
// CreateRepoWebhook on an installation that predates the App requesting
// "Repository hooks: write" (manifest.go's own doc comment). GitHub
// signals this with 403 Forbidden, not a distinct error code; callers
// check errors.Is(err, ErrPermissionDenied) rather than a status code.
var ErrPermissionDenied = errors.New("githubapp: permission denied by github")

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

// Credentials is ExchangeManifestCode's result: the real App identity
// and its three secrets, plus a couple of non-secret display fields
// (Slug, HTMLURL) internal/api uses to build the immediate
// install-prompt redirect. The caller (internal/api's
// handleGitHubAppCallback) must encrypt ClientSecret/WebhookSecret/
// PrivateKeyPEM through internal/secrets.Manager before this value goes
// out of scope; this type itself does nothing to protect them, the same
// "plaintext exists only transiently in memory, on this one path" shape
// every other secret-bearing request/response type in this codebase
// already has (createBackupTargetRequest, internal/api's own
// SecretSetter callers).
type Credentials struct {
	AppID         int64
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	PrivateKeyPEM string
	Slug          string
	HTMLURL       string
}

// manifestConversionResponse mirrors the subset of GitHub's "Create a
// GitHub App from a manifest" response
// (POST /app-manifests/{code}/conversions) this client actually reads.
// GitHub's real response includes many more fields (permissions,
// events, owner, installations_count, ...); only fields this control
// plane persists are decoded, everything else is silently ignored by
// encoding/json, the standard "narrow decode struct" shape used
// throughout this codebase's other external-API clients
// (internal/backup's S3 response handling included).
type manifestConversionResponse struct {
	ID            int64  `json:"id"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
	Slug          string `json:"slug"`
	HTMLURL       string `json:"html_url"`
}

// ExchangeManifestCode exchanges the one-time code GitHub's manifest
// flow redirect carries for the newly created App's real credentials.
// Per GitHub's documented behavior for this endpoint, no Authorization
// header is sent: the code itself, freshly minted by GitHub and usable
// exactly once, is the only credential this call needs. If GitHub ever
// starts requiring one, adding a personal-access-token Authorization
// header here is the documented fix.
func (c *Client) ExchangeManifestCode(ctx context.Context, instanceURL, code string) (Credentials, error) {
	var resp manifestConversionResponse
	path := "/app-manifests/" + url.PathEscape(code) + "/conversions"
	if err := c.do(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, "", &resp); err != nil {
		return Credentials{}, err
	}
	return Credentials{
		AppID:         resp.ID,
		ClientID:      resp.ClientID,
		ClientSecret:  resp.ClientSecret,
		WebhookSecret: resp.WebhookSecret,
		PrivateKeyPEM: resp.PEM,
		Slug:          resp.Slug,
		HTMLURL:       resp.HTMLURL,
	}, nil
}

// InstallationInfo is GetInstallation's result: just enough of GitHub's
// Installation object to record who the App is installed for
// (AccountLogin) and to defend against a cross-App installation_id
// (AppID, checked by the caller against the App's own stored ID before
// trusting this installation at all: see handleGitHubAppInstalled's own
// doc comment for why that check exists). SuspendedAt is non-empty when
// an org admin suspended the installation without uninstalling it: GitHub
// keeps returning 200 for a suspended installation, only access tokens
// and API calls made with them start failing, so this is the only signal
// that distinguishes "suspended" from "installed" at this call.
type InstallationInfo struct {
	ID           int64
	AppID        int64
	AccountLogin string
	SuspendedAt  string
}

type installationResponse struct {
	ID    int64 `json:"id"`
	AppID int64 `json:"app_id"`
	// Account is either a user or an organization in GitHub's schema;
	// both shapes carry "login" at the top level, so decoding just that
	// one field works for either.
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
	SuspendedAt *string `json:"suspended_at"`
}

// ErrInstallationNotFound wraps the API error GitHub returns for an
// installation_id that no longer exists (the App was fully uninstalled),
// the GetInstallation counterpart to ErrPermissionDenied. GitHub signals
// this with 404 Not Found; callers check
// errors.Is(err, ErrInstallationNotFound) rather than a status code.
var ErrInstallationNotFound = errors.New("githubapp: installation not found")

// GetInstallation looks up an installation by ID, authenticated as the
// App itself (appJWT, from SignAppJWT) rather than as the installation:
// this is the one call in this client that happens before an
// installation access token exists to use instead.
func (c *Client) GetInstallation(ctx context.Context, instanceURL, appJWT string, installationID int64) (InstallationInfo, error) {
	var resp installationResponse
	path := "/app/installations/" + strconv.FormatInt(installationID, 10)
	if err := c.do(ctx, c.APIBaseURL(instanceURL), http.MethodGet, path, bearerPrefix+appJWT, &resp); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return InstallationInfo{}, fmt.Errorf("%w: %w", ErrInstallationNotFound, err)
		}
		return InstallationInfo{}, err
	}
	info := InstallationInfo{ID: resp.ID, AppID: resp.AppID, AccountLogin: resp.Account.Login}
	if resp.SuspendedAt != nil {
		info.SuspendedAt = *resp.SuspendedAt
	}
	return info, nil
}

// InstallationToken is MintInstallationToken's result: a short-lived
// (~1 hour per GitHub's docs) credential scoped to exactly one
// installation. internal/api mints a fresh one per repo/branch-listing
// request rather than caching and reusing one across requests: see
// ListInstallationRepos/ListBranches's own callers in internal/api for
// why that's the deliberate, simpler choice here.
type InstallationToken struct {
	Token     string
	ExpiresAt time.Time
}

type installationTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// MintInstallationToken exchanges an App-level JWT for a short-lived
// installation access token, authenticated as the App
// (Authorization: Bearer <appJWT>), the same scheme GetInstallation
// uses and the last call in this client that needs the App JWT rather
// than the resulting installation token.
func (c *Client) MintInstallationToken(ctx context.Context, instanceURL, appJWT string, installationID int64) (InstallationToken, error) {
	var resp installationTokenResponse
	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := c.do(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, bearerPrefix+appJWT, &resp); err != nil {
		return InstallationToken{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	if err != nil {
		return InstallationToken{}, fmt.Errorf("githubapp: parse installation token expires_at %q: %w", resp.ExpiresAt, err)
	}
	return InstallationToken{Token: resp.Token, ExpiresAt: expiresAt}, nil
}

// Repo is one repository this control plane's GitHub App installation
// can access, the subset of GitHub's Repository object internal/api's
// repo picker actually needs.
type Repo struct {
	FullName      string
	Name          string
	OwnerLogin    string
	Private       bool
	DefaultBranch string
	HTMLURL       string
}

type repoResponse struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

type listRepositoriesResponse struct {
	TotalCount   int            `json:"total_count"`
	Repositories []repoResponse `json:"repositories"`
}

// listPerPage is the page size used for every paginated list call this
// client makes: GitHub's own documented maximum, so pagination finishes
// in as few round trips as possible.
const listPerPage = 100

// listPageCap bounds how many pages ListInstallationRepos/ListBranches
// will ever fetch (100 * 20 = 2000 items), a safety valve against an
// unbounded loop if GitHub's pagination signal is ever misread; no
// operator-realistic installation or repository is expected to actually
// hit this.
const listPageCap = 20

// ListInstallationRepos lists every repository accessible to token
// (an installation access token from MintInstallationToken), across as
// many pages as GitHub returns full pages for.
func (c *Client) ListInstallationRepos(ctx context.Context, instanceURL, token string) ([]Repo, error) {
	baseURL := c.APIBaseURL(instanceURL)
	var out []Repo
	for page := 1; page <= listPageCap; page++ {
		var resp listRepositoriesResponse
		path := fmt.Sprintf("/installation/repositories?per_page=%d&page=%d", listPerPage, page)
		if err := c.do(ctx, baseURL, http.MethodGet, path, bearerPrefix+token, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Repositories {
			out = append(out, Repo{
				FullName:      r.FullName,
				Name:          r.Name,
				OwnerLogin:    r.Owner.Login,
				Private:       r.Private,
				DefaultBranch: r.DefaultBranch,
				HTMLURL:       r.HTMLURL,
			})
		}
		if len(resp.Repositories) < listPerPage {
			break
		}
	}
	return out, nil
}

// GetRepo looks up a single repository by owner/repo, authenticated
// with an installation access token the same way ListInstallationRepos
// is.
func (c *Client) GetRepo(ctx context.Context, instanceURL, token, owner, repo string) (Repo, error) {
	var resp repoResponse
	baseURL := c.APIBaseURL(instanceURL)
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.do(ctx, baseURL, http.MethodGet, path, bearerPrefix+token, &resp); err != nil {
		return Repo{}, err
	}
	return Repo{
		FullName:      resp.FullName,
		Name:          resp.Name,
		OwnerLogin:    resp.Owner.Login,
		Private:       resp.Private,
		DefaultBranch: resp.DefaultBranch,
		HTMLURL:       resp.HTMLURL,
	}, nil
}

// Branch is one branch of one repository.
type Branch struct {
	Name      string
	CommitSHA string
}

type branchResponse struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// ListBranches lists every branch of owner/repo, authenticated with an
// installation access token the same way ListInstallationRepos is.
func (c *Client) ListBranches(ctx context.Context, instanceURL, token, owner, repo string) ([]Branch, error) {
	baseURL := c.APIBaseURL(instanceURL)
	var out []Branch
	for page := 1; page <= listPageCap; page++ {
		var resp []branchResponse
		path := fmt.Sprintf("/repos/%s/%s/branches?per_page=%d&page=%d",
			url.PathEscape(owner), url.PathEscape(repo), listPerPage, page)
		if err := c.do(ctx, baseURL, http.MethodGet, path, bearerPrefix+token, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp {
			out = append(out, Branch{Name: b.Name, CommitSHA: b.Commit.SHA})
		}
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

type createRepoHookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret"`
}

type createRepoHookRequest struct {
	Name   string               `json:"name"`
	Active bool                 `json:"active"`
	Events []string             `json:"events"`
	Config createRepoHookConfig `json:"config"`
}

// CreateRepoWebhook registers a push webhook on owner/repo pointed at
// hookURL, authenticated with an installation access token. Distinct
// from the App's own hook_attributes (see this package's own doc
// comment): that's a single, App-wide webhook GitHub never delivers to
// (Active: false); this is a real, per-repo classic webhook, the same
// mechanism GitLab's and Bitbucket's own CreateProjectWebhook/
// CreateRepoWebhook use. Requires "Repository hooks: write"
// (manifest.go's DefaultManifestConfig); on an installation that
// predates that permission, GitHub rejects this with 403
// (ErrPermissionDenied), which callers should degrade around rather
// than treat as fatal.
func (c *Client) CreateRepoWebhook(ctx context.Context, instanceURL, token, owner, repo, hookURL, secret string) error {
	body, err := json.Marshal(createRepoHookRequest{ //nolint:gosec // secret is sent to GitHub to configure delivery signing, not a leaked credential
		Name:   "web",
		Active: true,
		Events: []string{"push"},
		Config: createRepoHookConfig{URL: hookURL, ContentType: "json", Secret: secret},
	})
	if err != nil {
		return fmt.Errorf("githubapp: marshal webhook request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/hooks", url.PathEscape(owner), url.PathEscape(repo))
	return c.doWithBody(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, bearerPrefix+token, body)
}

// Package azuredevops is a small client for Azure DevOps Services and Server
// (REST api 7.1), authenticated with a personal access token. An instance URL
// is the organization root, https://dev.azure.com/ORG, and repositories are
// addressed as "PROJECT/repo".
package azuredevops

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// Client talks to one organization per call; the URL is an argument because
// Azure DevOps Server installs are self-hosted.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client on the SSRF-guarded HTTP client, since instance URLs are operator supplied.
func NewClient() *Client {
	return &Client{HTTP: gitprovider.NewGuardedClient()}
}

const (
	errPrefix  = "azuredevops"
	apiName    = "azure-devops"
	apiVersion = "7.1"
	// WebhookUser is the basic-auth user the service hook sends; the password is the shared secret.
	WebhookUser = "webhook"
)

func apiURL(instanceURL, path string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	q.Set("api-version", apiVersion)
	return strings.TrimRight(instanceURL, "/") + path + "?" + q.Encode()
}

// authHeader encodes a PAT as basic auth with an empty user, the form Azure DevOps expects.
func authHeader(token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+token))
}

func (c *Client) do(ctx context.Context, method, fullURL, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", errPrefix, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", authHeader(token))
	return gitprovider.Execute(c.HTTP, req, errPrefix, apiName, method+" "+req.URL.Redacted(), out)
}

// SplitRepo parses "PROJECT/repo" into its two names.
func SplitRepo(repo string) (project, name string, err error) {
	project, name, ok := strings.Cut(strings.Trim(repo, "/"), "/")
	if !ok || project == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("%s: repo must be PROJECT/repo, got %q", errPrefix, repo)
	}
	return project, name, nil
}

func repoPath(repo string) (string, error) {
	project, name, err := SplitRepo(repo)
	if err != nil {
		return "", err
	}
	return "/" + url.PathEscape(project) + "/_apis/git/repositories/" + url.PathEscape(name), nil
}

// Account is the user behind a personal access token.
type Account struct {
	ID    string
	Login string
}

// CurrentUser resolves the token's user from the organization's connectionData.
// The id is prefixed with the organization so one person can connect several orgs.
func (c *Client) CurrentUser(ctx context.Context, instanceURL, token string) (Account, error) {
	var resp struct {
		AuthenticatedUser struct {
			ID                  string `json:"id"`
			ProviderDisplayName string `json:"providerDisplayName"`
		} `json:"authenticatedUser"`
	}
	q := url.Values{"connectOptions": {"none"}}
	if err := c.do(ctx, http.MethodGet, apiURL(instanceURL, "/_apis/connectionData", q), token, nil, &resp); err != nil {
		return Account{}, err
	}
	u := resp.AuthenticatedUser
	if u.ID == "" {
		return Account{}, &gitprovider.APIError{Prefix: errPrefix, API: apiName, StatusCode: http.StatusUnauthorized, Body: "connectionData returned no user"}
	}
	login := u.ProviderDisplayName
	if login == "" {
		login = u.ID
	}
	return Account{ID: OrgName(instanceURL) + "/" + u.ID, Login: login}, nil
}

// OrgName returns the organization segment of an instance URL, or the host when there is none.
func OrgName(instanceURL string) string {
	u, err := url.Parse(instanceURL)
	if err != nil {
		return ""
	}
	if seg := strings.Trim(u.Path, "/"); seg != "" {
		first, _, _ := strings.Cut(seg, "/")
		return first
	}
	return u.Host
}

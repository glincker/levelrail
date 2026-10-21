// Package bitbucketserver is a small client for Bitbucket Server and Data
// Center (REST /rest/api/1.0), authenticated with a personal access token.
// It is unrelated to Bitbucket Cloud (internal/bitbucketapp): different API,
// auth and webhook format. Repositories are addressed as "PROJECT/slug".
package bitbucketserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// Client talks to one Bitbucket Server instance per call; the instance URL is
// an argument because every install is self-hosted.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client on the SSRF-guarded HTTP client, since instance URLs are operator supplied.
func NewClient() *Client {
	return &Client{HTTP: gitprovider.NewGuardedClient()}
}

const (
	errPrefix = "bitbucketserver"
	apiName   = "bitbucket-server"
	pageLimit = 100
	pageCap   = 20
)

func apiBase(instanceURL string) string {
	return strings.TrimRight(instanceURL, "/") + "/rest/api/1.0"
}

func (c *Client) do(ctx context.Context, method, fullURL, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", errPrefix, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return gitprovider.Execute(c.HTTP, req, errPrefix, apiName, method+" "+req.URL.Redacted(), out)
}

// SplitRepo parses "PROJECT/slug" into its two keys.
func SplitRepo(repo string) (project, slug string, err error) {
	project, slug, ok := strings.Cut(strings.Trim(repo, "/"), "/")
	if !ok || project == "" || slug == "" || strings.Contains(slug, "/") {
		return "", "", fmt.Errorf("%s: repo must be PROJECT/slug, got %q", errPrefix, repo)
	}
	return project, slug, nil
}

func repoPath(instanceURL, repo string) (string, error) {
	project, slug, err := SplitRepo(repo)
	if err != nil {
		return "", err
	}
	return apiBase(instanceURL) + "/projects/" + url.PathEscape(project) + "/repos/" + url.PathEscape(slug), nil
}

// Account is the user behind a personal access token.
type Account struct {
	ID    string
	Login string
}

// CurrentUser resolves the token's user: whoami returns the login as plain
// text, then the users endpoint supplies the stable numeric id.
func (c *Client) CurrentUser(ctx context.Context, instanceURL, token string) (Account, error) {
	whoami := strings.TrimRight(instanceURL, "/") + "/plugins/servlet/applinks/whoami"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, whoami, nil)
	if err != nil {
		return Account{}, fmt.Errorf("%s: build request: %w", errPrefix, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req) //nolint:gosec // instance URL is operator configured and the client is SSRF guarded
	if err != nil {
		return Account{}, fmt.Errorf("%s: request whoami: %w", errPrefix, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	login := strings.TrimSpace(string(raw))
	if resp.StatusCode != http.StatusOK || login == "" {
		return Account{}, &gitprovider.APIError{Prefix: errPrefix, API: apiName, StatusCode: resp.StatusCode, Body: "whoami returned no user"}
	}
	var u struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodGet, apiBase(instanceURL)+"/users/"+url.PathEscape(login), token, nil, &u); err != nil {
		return Account{}, err
	}
	return Account{ID: fmt.Sprint(u.ID), Login: login}, nil
}

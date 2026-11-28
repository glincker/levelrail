package giteaapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/glincker/levelrail/internal/gitprovider"
)

// ListOrgs lists the organizations the token's user belongs to.
func (c *Client) ListOrgs(ctx context.Context, instanceURL, accessToken string) ([]string, error) {
	var out []string
	for page := 1; page <= listPageCap; page++ {
		var resp []struct {
			Username string `json:"username"`
		}
		u := fmt.Sprintf("%s/user/orgs?limit=%d&page=%d", apiBaseURL(instanceURL), listPerPage, page)
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, o := range resp {
			out = append(out, o.Username)
		}
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

// ListOwnerRepos lists the repositories of one owner, an organization or a
// user, so an import can be scoped to a single owner instead of everything
// the token can see. Organizations are tried first, users on a 404.
func (c *Client) ListOwnerRepos(ctx context.Context, instanceURL, accessToken, owner string, opts gitprovider.ListOptions) ([]Repo, gitprovider.ListResult, error) {
	repos, res, err := c.listOwnerRepos(ctx, instanceURL, accessToken, "orgs", owner, opts)
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return c.listOwnerRepos(ctx, instanceURL, accessToken, "users", owner, opts)
	}
	return repos, res, err
}

func (c *Client) listOwnerRepos(ctx context.Context, instanceURL, accessToken, kind, owner string, opts gitprovider.ListOptions) ([]Repo, gitprovider.ListResult, error) {
	var (
		out []Repo
		res gitprovider.ListResult
	)
	for page := 1; ; page++ {
		var resp []repoResponse
		u := fmt.Sprintf("%s/%s/%s/repos?limit=%d&page=%d", apiBaseURL(instanceURL), kind, url.PathEscape(owner), listPerPage, page)
		err := gitprovider.WithRetry(ctx, opts.Retry, "gitea", func() error {
			resp = nil
			return c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp)
		})
		if err != nil {
			return nil, gitprovider.ListResult{}, err
		}
		for _, r := range resp {
			out = append(out, toRepo(r))
		}
		if len(resp) < listPerPage {
			break
		}
		if opts.Reached(len(out)) {
			res.Truncated = true
			break
		}
	}
	return out, res, nil
}

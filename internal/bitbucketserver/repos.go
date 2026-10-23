package bitbucketserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// Repo is one repository, reduced to what the picker and source setup need.
type Repo struct {
	FullName      string
	Name          string
	Private       bool
	DefaultBranch string
	CloneURL      string
	WebURL        string
}

type link struct {
	Href string `json:"href"`
	Name string `json:"name"`
}

type repoResponse struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Public   bool   `json:"public"`
	Archived bool   `json:"archived"`
	Project  struct {
		Key    string `json:"key"`
		Public bool   `json:"public"`
	} `json:"project"`
	Links struct {
		Clone []link `json:"clone"`
		Self  []link `json:"self"`
	} `json:"links"`
}

type page[T any] struct {
	Values        []T  `json:"values"`
	IsLastPage    bool `json:"isLastPage"`
	NextPageStart int  `json:"nextPageStart"`
}

func toRepo(r repoResponse) Repo {
	out := Repo{FullName: r.Project.Key + "/" + r.Slug, Name: r.Name, Private: !r.Public && !r.Project.Public}
	for _, l := range r.Links.Clone {
		if l.Name == "http" || l.Name == "https" {
			out.CloneURL = stripUserinfo(l.Href)
			break
		}
	}
	if len(r.Links.Self) > 0 {
		out.WebURL = r.Links.Self[0].Href
	}
	return out
}

// stripUserinfo drops the "user@" Bitbucket Server puts in HTTP clone links, so no login ends up in a stored source URL.
func stripUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// ListRepos lists repositories the token can see, following nextPageStart until the last page or MaxItems.
func (c *Client) ListRepos(ctx context.Context, instanceURL, token string, opts gitprovider.ListOptions) ([]Repo, gitprovider.ListResult, error) {
	var (
		out   []Repo
		res   gitprovider.ListResult
		start int
	)
	for i := 0; i < pageCap; i++ {
		var resp page[repoResponse]
		u := fmt.Sprintf("%s/repos?limit=%d&start=%d", apiBase(instanceURL), pageLimit, start)
		err := gitprovider.WithRetry(ctx, opts.Retry, apiName, func() error {
			resp = page[repoResponse]{}
			return c.do(ctx, http.MethodGet, u, token, nil, &resp)
		})
		if err != nil {
			return nil, gitprovider.ListResult{}, err
		}
		for _, r := range resp.Values {
			if !r.Archived {
				out = append(out, toRepo(r))
			}
		}
		if resp.IsLastPage {
			return out, res, nil
		}
		if opts.Reached(len(out)) {
			break
		}
		start = resp.NextPageStart
	}
	res.Truncated = true
	return out, res, nil
}

// GetRepo looks up "PROJECT/slug" and asks the server for its default branch.
func (c *Client) GetRepo(ctx context.Context, instanceURL, token, repo string) (Repo, error) {
	base, err := repoPath(instanceURL, repo)
	if err != nil {
		return Repo{}, err
	}
	var resp repoResponse
	if err := c.do(ctx, http.MethodGet, base, token, nil, &resp); err != nil {
		return Repo{}, err
	}
	out := toRepo(resp)
	out.DefaultBranch, err = c.DefaultBranch(ctx, instanceURL, token, repo)
	return out, err
}

// DefaultBranch returns the repository's default branch, or "" for an empty repository.
func (c *Client) DefaultBranch(ctx context.Context, instanceURL, token, repo string) (string, error) {
	base, err := repoPath(instanceURL, repo)
	if err != nil {
		return "", err
	}
	var resp struct {
		DisplayID string `json:"displayId"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/default-branch", token, nil, &resp); err != nil {
		var ae *gitprovider.APIError
		if errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound {
			return "", nil
		}
		return "", err
	}
	return resp.DisplayID, nil
}

// Branch is one branch and its head commit.
type Branch struct {
	Name      string
	CommitSHA string
}

// ListBranches lists the branches of "PROJECT/slug".
func (c *Client) ListBranches(ctx context.Context, instanceURL, token, repo string) ([]Branch, error) {
	base, err := repoPath(instanceURL, repo)
	if err != nil {
		return nil, err
	}
	var out []Branch
	start := 0
	for i := 0; i < pageCap; i++ {
		var resp page[struct {
			DisplayID    string `json:"displayId"`
			LatestCommit string `json:"latestCommit"`
		}]
		u := fmt.Sprintf("%s/branches?limit=%d&start=%d", base, pageLimit, start)
		if err := c.do(ctx, http.MethodGet, u, token, nil, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp.Values {
			out = append(out, Branch{Name: b.DisplayID, CommitSHA: b.LatestCommit})
		}
		if resp.IsLastPage {
			break
		}
		start = resp.NextPageStart
	}
	return out, nil
}

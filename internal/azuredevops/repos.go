package azuredevops

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// Repo is one repository, reduced to what the picker and source setup need.
type Repo struct {
	ID            string
	FullName      string
	Name          string
	Private       bool
	DefaultBranch string
	CloneURL      string
	WebURL        string
	// ProjectID is the project GUID, which service hook subscriptions require.
	ProjectID string
}

type repoResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	RemoteURL     string `json:"remoteUrl"`
	WebURL        string `json:"webUrl"`
	IsDisabled    bool   `json:"isDisabled"`
	Project       struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
	} `json:"project"`
}

func toRepo(r repoResponse) Repo {
	return Repo{
		ID: r.ID, FullName: r.Project.Name + "/" + r.Name, Name: r.Name, Private: !strings.EqualFold(r.Project.Visibility, "public"),
		DefaultBranch: strings.TrimPrefix(r.DefaultBranch, "refs/heads/"),
		CloneURL:      stripUserinfo(r.RemoteURL), WebURL: r.WebURL, ProjectID: r.Project.ID,
	}
}

// stripUserinfo drops the "org@" Azure DevOps puts in clone URLs, so no login ends up in a stored source URL.
func stripUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// ListRepos lists the repositories of every project in the organization. The
// endpoint is not paged, so MaxItems only truncates the result.
func (c *Client) ListRepos(ctx context.Context, instanceURL, token string, opts gitprovider.ListOptions) ([]Repo, gitprovider.ListResult, error) {
	var resp struct {
		Value []repoResponse `json:"value"`
	}
	u := apiURL(instanceURL, "/_apis/git/repositories", nil)
	err := gitprovider.WithRetry(ctx, opts.Retry, apiName, func() error {
		resp.Value = nil
		return c.do(ctx, http.MethodGet, u, token, nil, &resp)
	})
	if err != nil {
		return nil, gitprovider.ListResult{}, err
	}
	var (
		out []Repo
		res gitprovider.ListResult
	)
	for _, r := range resp.Value {
		if r.IsDisabled {
			continue
		}
		if opts.Reached(len(out)) {
			res.Truncated = true
			break
		}
		out = append(out, toRepo(r))
	}
	return out, res, nil
}

// GetRepo looks up "PROJECT/repo".
func (c *Client) GetRepo(ctx context.Context, instanceURL, token, repo string) (Repo, error) {
	path, err := repoPath(repo)
	if err != nil {
		return Repo{}, err
	}
	var resp repoResponse
	if err := c.do(ctx, http.MethodGet, apiURL(instanceURL, path, nil), token, nil, &resp); err != nil {
		return Repo{}, err
	}
	return toRepo(resp), nil
}

// Branch is one branch and its head commit.
type Branch struct {
	Name      string
	CommitSHA string
}

// ListBranches lists the branches of "PROJECT/repo".
func (c *Client) ListBranches(ctx context.Context, instanceURL, token, repo string) ([]Branch, error) {
	path, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Value []struct {
			Name     string `json:"name"`
			ObjectID string `json:"objectId"`
		} `json:"value"`
	}
	u := apiURL(instanceURL, path+"/refs", url.Values{"filter": {"heads/"}})
	if err := c.do(ctx, http.MethodGet, u, token, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Branch, 0, len(resp.Value))
	for _, b := range resp.Value {
		out = append(out, Branch{Name: strings.TrimPrefix(b.Name, "refs/heads/"), CommitSHA: b.ObjectID})
	}
	return out, nil
}

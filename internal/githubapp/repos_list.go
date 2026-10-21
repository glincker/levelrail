package githubapp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// ListOptions bounds ListInstallationRepos. PriorETags and PriorRepos are
// the previous result's page ETags and repos: a page GitHub answers with
// 304 is reused from PriorRepos (a 304 does not count against the rate limit).
type ListOptions struct {
	gitprovider.ListOptions
	PriorETags  []string
	PriorRepos  []Repo
	MaxPageSize int
}

// ListResult reports how a listing ended.
type ListResult struct {
	// Truncated is true when the cap stopped paging while GitHub still had more.
	Truncated bool
	// ETags holds one entry per fetched page, in order, for the next call.
	ETags []string
	// NotModified counts pages served from PriorRepos.
	NotModified int
}

// ListInstallationRepos lists repositories accessible to token (an
// installation access token), page by page until GitHub runs out or the
// MaxItems cap is reached.
func (c *Client) ListInstallationRepos(ctx context.Context, instanceURL, token string, opts ListOptions) ([]Repo, ListResult, error) {
	baseURL := c.APIBaseURL(instanceURL)
	perPage := listPerPage
	if opts.MaxPageSize > 0 && opts.MaxPageSize < perPage {
		perPage = opts.MaxPageSize
	}
	var (
		out []Repo
		res ListResult
	)
	for page := 1; ; page++ {
		path := fmt.Sprintf("/installation/repositories?per_page=%d&page=%d", perPage, page)
		var (
			resp        listRepositoriesResponse
			etag        string
			notModified bool
		)
		prior := priorEntry(opts.PriorETags, page)
		err := gitprovider.WithRetry(ctx, opts.Retry, "github", func() error {
			req, reqErr := c.newGet(ctx, baseURL, path, bearerPrefix+token)
			if reqErr != nil {
				return reqErr
			}
			var doErr error
			etag, notModified, doErr = gitprovider.ExecuteETag(c.HTTP, req, errPrefix, apiName, http.MethodGet+" "+path, priorPageETag(prior, opts.PriorRepos, page, perPage), &resp)
			return doErr
		})
		if err != nil {
			return nil, ListResult{}, err
		}
		res.ETags = append(res.ETags, etag)

		var got int
		if notModified {
			res.NotModified++
			pageRepos := priorPage(opts.PriorRepos, page, perPage)
			out = append(out, pageRepos...)
			got = len(pageRepos)
		} else {
			for _, r := range resp.Repositories {
				out = append(out, toRepo(r))
			}
			got = len(resp.Repositories)
		}
		if got < perPage {
			break
		}
		if opts.Reached(len(out)) {
			res.Truncated = true
			if !notModified && resp.TotalCount > 0 {
				res.Truncated = resp.TotalCount > len(out)
			}
			break
		}
	}
	return out, res, nil
}

func (c *Client) newGet(ctx context.Context, baseURL, path, authHeader string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", errPrefix, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("Authorization", authHeader)
	return req, nil
}

func priorEntry(etags []string, page int) string {
	if page-1 < len(etags) {
		return etags[page-1]
	}
	return ""
}

// priorPageETag returns the ETag to send for page only when the prior
// result still holds that whole page.
func priorPageETag(etag string, priorRepos []Repo, page, perPage int) string {
	if etag == "" || len(priorPage(priorRepos, page, perPage)) == 0 {
		return ""
	}
	return etag
}

func priorPage(repos []Repo, page, perPage int) []Repo {
	start := (page - 1) * perPage
	if start >= len(repos) {
		return nil
	}
	return repos[start:min(start+perPage, len(repos))]
}

func toRepo(r repoResponse) Repo {
	return Repo{
		FullName:      r.FullName,
		Name:          r.Name,
		OwnerLogin:    r.Owner.Login,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		HTMLURL:       r.HTMLURL,
		Language:      r.Language,
		PushedAt:      r.PushedAt,
		Archived:      r.Archived,
		Fork:          r.Fork,
	}
}

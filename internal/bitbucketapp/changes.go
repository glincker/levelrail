package bitbucketapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// BuildStatusStopped marks a build status as stopped before completing.
const BuildStatusStopped BuildStatusState = "STOPPED"

type diffstatResponse struct {
	Values []struct {
		Old *struct {
			Path string `json:"path"`
		} `json:"old"`
		New *struct {
			Path string `json:"path"`
		} `json:"new"`
	} `json:"values"`
	Next string `json:"next"`
}

func (c *Client) diffstatFiles(ctx context.Context, accessToken, first string) ([]string, error) {
	var out []string
	next := first
	for page := 0; page < listPageCap && next != ""; page++ {
		var resp diffstatResponse
		if err := c.do(ctx, http.MethodGet, next, bearerPrefix+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		for _, v := range resp.Values {
			if v.New != nil {
				out = append(out, v.New.Path)
			}
			if v.Old != nil && (v.New == nil || v.Old.Path != v.New.Path) {
				out = append(out, v.Old.Path)
			}
		}
		next = resp.Next
	}
	return out, nil
}

// CompareChangedFiles lists the files that differ between head and base.
func (c *Client) CompareChangedFiles(ctx context.Context, accessToken, fullName, head, base string) ([]string, error) {
	spec := url.PathEscape(head + ".." + base)
	return c.diffstatFiles(ctx, accessToken, c.apiBaseURL()+repositoriesAPI+fullName+"/diffstat/"+spec+"?pagelen=100")
}

// PullRequestChangedFiles lists the files changed by pull request prID.
func (c *Client) PullRequestChangedFiles(ctx context.Context, accessToken, fullName string, prID int) ([]string, error) {
	return c.diffstatFiles(ctx, accessToken, c.apiBaseURL()+repositoriesAPI+fullName+"/pullrequests/"+strconv.Itoa(prID)+"/diffstat?pagelen=100")
}

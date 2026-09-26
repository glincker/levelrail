package giteaapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// CommitStatusError marks a commit status as errored.
const CommitStatusError CommitStatusState = "error"

type changedFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
}

func collectFilenames(files []changedFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Filename)
		if f.PreviousFilename != "" {
			out = append(out, f.PreviousFilename)
		}
	}
	return out
}

// PullRequestChangedFiles lists the files changed by pull request number.
func (c *Client) PullRequestChangedFiles(ctx context.Context, instanceURL, accessToken, fullName string, number int) ([]string, error) {
	var out []string
	for page := 1; page <= listPageCap; page++ {
		var resp []changedFile
		u := fmt.Sprintf("%s/repos/%s/pulls/%d/files?limit=%d&page=%d", apiBaseURL(instanceURL), fullName, number, listPerPage, page)
		if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, collectFilenames(resp)...)
		if len(resp) < listPerPage {
			break
		}
	}
	return out, nil
}

// CompareChangedFiles lists the files touched by the commits between base
// and head.
func (c *Client) CompareChangedFiles(ctx context.Context, instanceURL, accessToken, fullName, base, head string) ([]string, error) {
	var resp struct {
		Commits []struct {
			Files []changedFile `json:"files"`
		} `json:"commits"`
	}
	u := fmt.Sprintf("%s/repos/%s/compare/%s...%s", apiBaseURL(instanceURL), fullName, url.PathEscape(base), url.PathEscape(head))
	if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
		return nil, err
	}
	var out []string
	for _, cm := range resp.Commits {
		out = append(out, collectFilenames(cm.Files)...)
	}
	return out, nil
}

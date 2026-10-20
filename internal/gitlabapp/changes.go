package gitlabapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Extra commit states used for pipeline run reporting.
const (
	// CommitStateRunning marks a commit status as actively running.
	CommitStateRunning CommitState = "running"
	// CommitStateCanceled marks a commit status as canceled.
	CommitStateCanceled CommitState = "canceled"
)

type changedPath struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

func collectPaths(paths []changedPath) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, p.NewPath)
		if p.OldPath != "" && p.OldPath != p.NewPath {
			out = append(out, p.OldPath)
		}
	}
	return out
}

// CompareChangedFiles lists the files that differ between from and to.
func (c *Client) CompareChangedFiles(ctx context.Context, instanceURL, accessToken, projectPath, from, to string) ([]string, error) {
	var resp struct {
		Diffs []changedPath `json:"diffs"`
	}
	u := fmt.Sprintf("%s/projects/%s/repository/compare?from=%s&to=%s&straight=true",
		apiBaseURL(instanceURL), encodedProjectID(projectPath), url.QueryEscape(from), url.QueryEscape(to))
	if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
		return nil, err
	}
	return collectPaths(resp.Diffs), nil
}

// MergeRequestChangedFiles lists the files changed by merge request mrIID.
func (c *Client) MergeRequestChangedFiles(ctx context.Context, instanceURL, accessToken, projectPath string, mrIID int) ([]string, error) {
	var resp struct {
		Changes []changedPath `json:"changes"`
	}
	u := fmt.Sprintf("%s/projects/%s/merge_requests/%d/changes", apiBaseURL(instanceURL), encodedProjectID(projectPath), mrIID)
	if err := c.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &resp); err != nil {
		return nil, err
	}
	return collectPaths(resp.Changes), nil
}

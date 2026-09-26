package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// CommitStatusError marks a commit status as errored (the run could not
// complete, as opposed to a test failure).
const CommitStatusError CommitStatusState = "error"

// DeploymentState is a GitHub Deployments API status state.
type DeploymentState string

// Deployment states this client posts.
const (
	DeploymentInProgress DeploymentState = "in_progress"
	DeploymentSuccess    DeploymentState = "success"
	DeploymentFailure    DeploymentState = "failure"
	DeploymentInactive   DeploymentState = "inactive"
)

const changedFilesPageCap = 30

func (c *Client) doJSON(ctx context.Context, baseURL, method, path, authHeader string, body []byte, out any) error {
	var rd *bytes.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if rd != nil {
		req, err = http.NewRequestWithContext(ctx, method, baseURL+path, rd)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, baseURL+path, nil)
	}
	if err != nil {
		return fmt.Errorf("%s: build request: %w", errPrefix, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return gitprovider.Execute(c.HTTP, req, errPrefix, apiName, method+" "+path, out)
}

type createDeploymentRequest struct {
	Ref                   string   `json:"ref"`
	Environment           string   `json:"environment"`
	Description           string   `json:"description,omitempty"`
	AutoMerge             bool     `json:"auto_merge"`
	RequiredContexts      []string `json:"required_contexts"`
	TransientEnvironment  bool     `json:"transient_environment,omitempty"`
	ProductionEnvironment bool     `json:"production_environment"`
}

// CreateDeployment records a deployment of ref to environment and returns
// its ID. Required status contexts are skipped so the deployment never
// blocks on unrelated checks.
func (c *Client) CreateDeployment(ctx context.Context, instanceURL, token, owner, repo, ref, environment, description string, production bool) (int64, error) {
	payload, err := json.Marshal(createDeploymentRequest{
		Ref: ref, Environment: environment, Description: description,
		RequiredContexts: []string{}, TransientEnvironment: !production, ProductionEnvironment: production,
	})
	if err != nil {
		return 0, fmt.Errorf("githubapp: marshal deployment request: %w", err)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	path := fmt.Sprintf("/repos/%s/%s/deployments", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, bearerPrefix+token, payload, &resp); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("githubapp: create deployment: response carried no id")
	}
	return resp.ID, nil
}

type createDeploymentStatusRequest struct {
	State          string `json:"state"`
	EnvironmentURL string `json:"environment_url,omitempty"`
	LogURL         string `json:"log_url,omitempty"`
	Description    string `json:"description,omitempty"`
}

// CreateDeploymentStatus moves deployment id to state.
func (c *Client) CreateDeploymentStatus(ctx context.Context, instanceURL, token, owner, repo string, id int64, state DeploymentState, environmentURL, logURL, description string) error {
	payload, err := json.Marshal(createDeploymentStatusRequest{
		State: string(state), EnvironmentURL: environmentURL, LogURL: logURL, Description: description,
	})
	if err != nil {
		return fmt.Errorf("githubapp: marshal deployment status request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/deployments/%d/statuses", url.PathEscape(owner), url.PathEscape(repo), id)
	return c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, bearerPrefix+token, payload, nil)
}

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

// CompareChangedFiles lists the files that differ between base and head.
func (c *Client) CompareChangedFiles(ctx context.Context, instanceURL, token, owner, repo, base, head string) ([]string, error) {
	var out []string
	for page := 1; page <= changedFilesPageCap; page++ {
		var resp struct {
			Files []changedFile `json:"files"`
		}
		path := fmt.Sprintf("/repos/%s/%s/compare/%s...%s?per_page=100&page=%d",
			url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(base), url.PathEscape(head), page)
		if err := c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodGet, path, bearerPrefix+token, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, collectFilenames(resp.Files)...)
		if len(resp.Files) < 100 {
			break
		}
	}
	return out, nil
}

// PullRequestFiles lists the files changed by pull request number.
func (c *Client) PullRequestFiles(ctx context.Context, instanceURL, token, owner, repo string, number int) ([]string, error) {
	var out []string
	for page := 1; page <= changedFilesPageCap; page++ {
		var resp []changedFile
		path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files?per_page=100&page=%d", url.PathEscape(owner), url.PathEscape(repo), number, page)
		if err := c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodGet, path, bearerPrefix+token, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, collectFilenames(resp)...)
		if len(resp) < 100 {
			break
		}
	}
	return out, nil
}

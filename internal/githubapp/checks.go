package githubapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CheckRunSpec is the subset of a check run this client creates or updates.
type CheckRunSpec struct {
	Name       string
	HeadSHA    string
	Status     string // queued, in_progress, completed
	Conclusion string // required by GitHub once Status is completed
	DetailsURL string
	Title      string
	Summary    string
}

type checkRunOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type checkRunRequest struct {
	Name       string          `json:"name,omitempty"`
	HeadSHA    string          `json:"head_sha,omitempty"`
	Status     string          `json:"status"`
	Conclusion string          `json:"conclusion,omitempty"`
	DetailsURL string          `json:"details_url,omitempty"`
	Output     *checkRunOutput `json:"output,omitempty"`
}

func (s CheckRunSpec) request(includeIdentity bool) checkRunRequest {
	req := checkRunRequest{Status: s.Status, Conclusion: s.Conclusion, DetailsURL: s.DetailsURL}
	if includeIdentity {
		req.Name, req.HeadSHA = s.Name, s.HeadSHA
	}
	if s.Title != "" || s.Summary != "" {
		req.Output = &checkRunOutput{Title: s.Title, Summary: s.Summary}
	}
	return req
}

// CreateCheckRun opens a check run on spec.HeadSHA and returns its ID.
func (c *Client) CreateCheckRun(ctx context.Context, instanceURL, token, owner, repo string, spec CheckRunSpec) (int64, error) {
	payload, err := json.Marshal(spec.request(true))
	if err != nil {
		return 0, fmt.Errorf("githubapp: marshal check run request: %w", err)
	}
	var resp struct {
		ID int64 `json:"id"`
	}
	path := fmt.Sprintf("/repos/%s/%s/check-runs", url.PathEscape(owner), url.PathEscape(repo))
	if err := c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodPost, path, bearerPrefix+token, payload, &resp); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("githubapp: create check run: response carried no id")
	}
	return resp.ID, nil
}

// UpdateCheckRun moves check run id to spec's status and conclusion.
func (c *Client) UpdateCheckRun(ctx context.Context, instanceURL, token, owner, repo string, id int64, spec CheckRunSpec) error {
	payload, err := json.Marshal(spec.request(false))
	if err != nil {
		return fmt.Errorf("githubapp: marshal check run update: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/check-runs/%d", url.PathEscape(owner), url.PathEscape(repo), id)
	return c.doJSON(ctx, c.APIBaseURL(instanceURL), http.MethodPatch, path, bearerPrefix+token, payload, nil)
}

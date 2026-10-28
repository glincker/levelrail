package apiclient

import (
	"context"
	"net/http"
	"time"
)

// OneShotJobRequest is the body of POST /api/v1/jobs/runs.
type OneShotJobRequest struct {
	App            string   `json:"app"`
	Command        []string `json:"command"`
	Completions    int      `json:"completions,omitempty"`
	Parallelism    int      `json:"parallelism,omitempty"`
	BackoffLimit   *int     `json:"backoff_limit,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

// OneShotJob is a one-shot job's status.
type OneShotJob struct {
	ID             string     `json:"id"`
	App            string     `json:"app"`
	State          string     `json:"state"`
	Completions    int        `json:"completions"`
	Parallelism    int        `json:"parallelism"`
	BackoffLimit   int        `json:"backoff_limit"`
	Succeeded      int        `json:"succeeded"`
	FailedAttempts int        `json:"failed_attempts"`
	Active         int        `json:"active"`
	RunIDs         []string   `json:"run_ids"`
	Message        string     `json:"message,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// RunOneShotJob calls POST /api/v1/jobs/runs.
func (c *Client) RunOneShotJob(ctx context.Context, req OneShotJobRequest) (OneShotJob, error) {
	var out OneShotJob
	err := c.do(ctx, http.MethodPost, "/api/v1/jobs/runs", req, &out)
	return out, err
}

// GetOneShotJob calls GET /api/v1/jobs/runs/{id}.
func (c *Client) GetOneShotJob(ctx context.Context, id string) (OneShotJob, error) {
	var out OneShotJob
	err := c.do(ctx, http.MethodGet, "/api/v1/jobs/runs/"+PathEscape(id), nil, &out)
	return out, err
}

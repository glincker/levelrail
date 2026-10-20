package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

func jobsCollectionPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/jobs"
}

func jobPath(app, id string) string {
	return jobsCollectionPath(app) + "/" + PathEscape(id)
}

// CreateJob calls POST /api/v1/apps/{name}/jobs.
func (c *Client) CreateJob(ctx context.Context, app string, req ScheduledTaskRequest) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPost, jobsCollectionPath(app), req, &out)
	return out, err
}

// ListJobs calls GET /api/v1/apps/{name}/jobs.
func (c *Client) ListJobs(ctx context.Context, app string) ([]ScheduledTaskResource, error) {
	var out []ScheduledTaskResource
	err := c.do(ctx, http.MethodGet, jobsCollectionPath(app), nil, &out)
	return out, err
}

// GetJob calls GET /api/v1/apps/{name}/jobs/{id}; id may be the job's name.
func (c *Client) GetJob(ctx context.Context, app, id string) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodGet, jobPath(app, id), nil, &out)
	return out, err
}

// UpdateJob calls PUT /api/v1/apps/{name}/jobs/{id}. Fields the request
// leaves out keep their stored value.
func (c *Client) UpdateJob(ctx context.Context, app, id string, req ScheduledTaskRequest) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPut, jobPath(app, id), req, &out)
	return out, err
}

// DeleteJob calls DELETE /api/v1/apps/{name}/jobs/{id}.
func (c *Client) DeleteJob(ctx context.Context, app, id string) error {
	return c.do(ctx, http.MethodDelete, jobPath(app, id), nil, nil)
}

// SetJobEnabled calls POST .../jobs/{id}/enable or /disable.
func (c *Client) SetJobEnabled(ctx context.Context, app, id string, enabled bool) (ScheduledTaskResource, error) {
	verb := "/disable"
	if enabled {
		verb = "/enable"
	}
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPost, jobPath(app, id)+verb, nil, &out)
	return out, err
}

// RunJob calls POST .../jobs/{id}/run and returns the started run's ID in
// the response's RunID.
func (c *Client) RunJob(ctx context.Context, app, id string) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPost, jobPath(app, id)+"/run", nil, &out)
	return out, err
}

// ListJobRuns calls GET .../jobs/{id}/runs, or the app-wide
// GET .../job-runs when id is empty. status and limit are optional filters.
func (c *Client) ListJobRuns(ctx context.Context, app, id, status string, limit int) ([]JobRunResource, error) {
	path := "/api/v1/apps/" + PathEscape(app) + "/job-runs"
	if id != "" {
		path = jobPath(app, id) + "/runs"
	}
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []JobRunResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetJobRun calls GET .../jobs/{id}/runs/{run}, including the output.
func (c *Client) GetJobRun(ctx context.Context, app, id, runID string) (JobRunResource, error) {
	var out JobRunResource
	err := c.do(ctx, http.MethodGet, jobPath(app, id)+"/runs/"+PathEscape(runID), nil, &out)
	return out, err
}

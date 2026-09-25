package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PipelineResource mirrors internal/api's pipelineResource.
type PipelineResource struct {
	ID        string    `json:"id"`
	App       string    `json:"app"`
	Name      string    `json:"name"`
	Source    string    `json:"source"`
	Enabled   bool      `json:"enabled"`
	YAML      string    `json:"yaml,omitempty"`
	Triggers  []string  `json:"triggers,omitempty"`
	Jobs      int       `json:"jobs"`
	LastRun   *RunBrief `json:"last_run,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RunBrief is a pipeline's most recent run summary.
type RunBrief struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Status string `json:"status"`
}

// PipelineIssue is one validation problem in a pipeline definition.
type PipelineIssue struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// PipelineValidation is the result of POST /api/v1/pipelines/validate.
type PipelineValidation struct {
	Valid    bool            `json:"valid"`
	Issues   []PipelineIssue `json:"issues"`
	Triggers []string        `json:"triggers,omitempty"`
	Jobs     int             `json:"jobs,omitempty"`
}

// PipelineRunResource mirrors internal/api's pipelineRunResource.
type PipelineRunResource struct {
	ID           string             `json:"id"`
	PipelineID   string             `json:"pipeline_id"`
	PipelineName string             `json:"pipeline_name"`
	App          string             `json:"app"`
	Number       int                `json:"number"`
	Trigger      string             `json:"trigger"`
	Actor        string             `json:"actor,omitempty"`
	Ref          string             `json:"ref,omitempty"`
	CommitSHA    string             `json:"commit_sha,omitempty"`
	Inputs       map[string]string  `json:"inputs,omitempty"`
	Status       string             `json:"status"`
	Reason       string             `json:"reason,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	StartedAt    *time.Time         `json:"started_at,omitempty"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty"`
	Jobs         []PipelineJobView  `json:"jobs,omitempty"`
	Approvals    []PipelineApproval `json:"approvals,omitempty"`
}

// PipelineJobView is one job of a run with its steps.
type PipelineJobView struct {
	Key        string             `json:"key"`
	Name       string             `json:"name"`
	Stage      string             `json:"stage,omitempty"`
	Needs      []string           `json:"needs"`
	Matrix     map[string]string  `json:"matrix,omitempty"`
	NodeID     string             `json:"node_id,omitempty"`
	Status     string             `json:"status"`
	Reason     string             `json:"reason,omitempty"`
	Attempt    int                `json:"attempt"`
	Outputs    map[string]string  `json:"outputs,omitempty"`
	StartedAt  *time.Time         `json:"started_at,omitempty"`
	FinishedAt *time.Time         `json:"finished_at,omitempty"`
	Steps      []PipelineStepView `json:"steps"`
}

// PipelineStepView is one step of a job.
type PipelineStepView struct {
	Index      int        `json:"index"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Attempt    int        `json:"attempt"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// PipelineApproval is one manual approval gate of a run.
type PipelineApproval struct {
	ID              int64      `json:"id"`
	Job             string     `json:"job"`
	StepIndex       int        `json:"step_index"`
	Message         string     `json:"message"`
	RequiredAbility string     `json:"required_ability"`
	Decision        string     `json:"decision,omitempty"`
	DecidedBy       string     `json:"decided_by,omitempty"`
	Comment         string     `json:"comment,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
}

// PipelineLogLine is one output line of a run.
type PipelineLogLine struct {
	ID     int64     `json:"id"`
	Job    string    `json:"job"`
	Step   int       `json:"step"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
	Time   time.Time `json:"time"`
}

// PipelineSaveRequest is the body of create and update.
type PipelineSaveRequest struct {
	Name    string `json:"name,omitempty"`
	YAML    string `json:"yaml"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// PipelineStartRequest is the body of a manual or API run.
type PipelineStartRequest struct {
	Ref    string            `json:"ref,omitempty"`
	SHA    string            `json:"sha,omitempty"`
	Inputs map[string]string `json:"inputs,omitempty"`
}

// BrandShortName reads the control plane's brand short name from the public
// GET /api/v1/brand, used to find the branded pipeline directory in a repo.
func (c *Client) BrandShortName(ctx context.Context) (string, error) {
	var out struct {
		ShortName string `json:"ShortName"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/brand", nil, &out); err != nil {
		return "", err
	}
	return out.ShortName, nil
}

func pipelinePath(app string, parts ...string) string {
	p := "/api/v1/apps/" + PathEscape(app)
	for _, s := range parts {
		p += "/" + PathEscape(s)
	}
	return p
}

// ListPipelines calls GET /api/v1/apps/{app}/pipelines.
func (c *Client) ListPipelines(ctx context.Context, app string) ([]PipelineResource, error) {
	var out []PipelineResource
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipelines"), nil, &out)
}

// GetPipeline calls GET /api/v1/apps/{app}/pipelines/{name}.
func (c *Client) GetPipeline(ctx context.Context, app, name string) (PipelineResource, error) {
	var out PipelineResource
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipelines", name), nil, &out)
}

// CreatePipeline calls POST /api/v1/apps/{app}/pipelines.
func (c *Client) CreatePipeline(ctx context.Context, app string, req PipelineSaveRequest) (PipelineResource, error) {
	var out PipelineResource
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipelines"), req, &out)
}

// UpdatePipeline calls PUT /api/v1/apps/{app}/pipelines/{name}.
func (c *Client) UpdatePipeline(ctx context.Context, app, name string, req PipelineSaveRequest) (PipelineResource, error) {
	var out PipelineResource
	return out, c.do(ctx, http.MethodPut, pipelinePath(app, "pipelines", name), req, &out)
}

// DeletePipeline calls DELETE /api/v1/apps/{app}/pipelines/{name}.
func (c *Client) DeletePipeline(ctx context.Context, app, name string) error {
	return c.do(ctx, http.MethodDelete, pipelinePath(app, "pipelines", name), nil, nil)
}

// ValidatePipeline calls POST /api/v1/pipelines/validate.
func (c *Client) ValidatePipeline(ctx context.Context, yamlText string) (PipelineValidation, error) {
	var out PipelineValidation
	return out, c.do(ctx, http.MethodPost, "/api/v1/pipelines/validate", PipelineSaveRequest{YAML: yamlText}, &out)
}

// StartPipelineRun calls POST /api/v1/apps/{app}/pipelines/{name}/runs.
func (c *Client) StartPipelineRun(ctx context.Context, app, name string, req PipelineStartRequest) (PipelineRunResource, error) {
	var out PipelineRunResource
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipelines", name, "runs"), req, &out)
}

// ListPipelineRuns calls GET /api/v1/apps/{app}/pipeline-runs.
func (c *Client) ListPipelineRuns(ctx context.Context, app, pipeline string, limit int) ([]PipelineRunResource, error) {
	q := url.Values{}
	if pipeline != "" {
		q.Set("pipeline", pipeline)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := pipelinePath(app, "pipeline-runs")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []PipelineRunResource
	return out, c.do(ctx, http.MethodGet, path, nil, &out)
}

// GetPipelineRun calls GET /api/v1/apps/{app}/pipeline-runs/{id}.
func (c *Client) GetPipelineRun(ctx context.Context, app, id string) (PipelineRunResource, error) {
	var out PipelineRunResource
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipeline-runs", id), nil, &out)
}

// ListPipelineRunLogs calls GET .../pipeline-runs/{id}/logs. job filters to
// one job key and after resumes past a line ID.
func (c *Client) ListPipelineRunLogs(ctx context.Context, app, id, job string, after int64, limit int) ([]PipelineLogLine, error) {
	q := url.Values{}
	if job != "" {
		q.Set("job", job)
	}
	if after > 0 {
		q.Set("after", strconv.FormatInt(after, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := pipelinePath(app, "pipeline-runs", id, "logs")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []PipelineLogLine
	return out, c.do(ctx, http.MethodGet, path, nil, &out)
}

// StreamPipelineRunLogs follows GET .../pipeline-runs/{id}/logs/stream until
// the run finishes or ctx is cancelled.
func (c *Client) StreamPipelineRunLogs(ctx context.Context, app, id, job string, onLine func(PipelineLogLine) error) error {
	path := pipelinePath(app, "pipeline-runs", id, "logs", "stream")
	if job != "" {
		path += "?job=" + url.QueryEscape(job)
	}
	return streamSSE(ctx, c, path, func(l PipelineLogLine) error {
		if l.ID == 0 {
			return nil
		}
		return onLine(l)
	})
}

// CancelPipelineRun calls POST .../pipeline-runs/{id}/cancel.
func (c *Client) CancelPipelineRun(ctx context.Context, app, id string) error {
	return c.do(ctx, http.MethodPost, pipelinePath(app, "pipeline-runs", id, "cancel"), nil, nil)
}

// RerunPipelineRun calls POST .../pipeline-runs/{id}/rerun.
func (c *Client) RerunPipelineRun(ctx context.Context, app, id string) (PipelineRunResource, error) {
	var out PipelineRunResource
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipeline-runs", id, "rerun"), nil, &out)
}

// DecidePipelineApproval calls POST .../pipeline-runs/{id}/approvals/{approval}.
func (c *Client) DecidePipelineApproval(ctx context.Context, app, runID string, approvalID int64, decision, comment string) error {
	body := map[string]string{"decision": decision, "comment": comment}
	return c.do(ctx, http.MethodPost, pipelinePath(app, "pipeline-runs", runID, "approvals", fmt.Sprint(approvalID)), body, nil)
}

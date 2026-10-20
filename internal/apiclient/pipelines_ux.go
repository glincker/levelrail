package apiclient

import (
	"context"
	"net/http"
)

// PipelineFix is a typed suggestion for a failed job.
type PipelineFix struct {
	Code   string `json:"code"`
	Kind   string `json:"kind"`
	Job    string `json:"job"`
	Target string `json:"target,omitempty"`
	Hint   string `json:"hint"`
	Match  string `json:"match"`
}

// PipelineFixResult is the outcome of applying a fix.
type PipelineFixResult struct {
	Pipeline string `json:"pipeline"`
	Run      string `json:"run,omitempty"`
}

// PipelineJobInsight is one job's stats over recent runs.
type PipelineJobInsight struct {
	Job         string  `json:"job"`
	Runs        int     `json:"runs"`
	Failures    int     `json:"failures"`
	FailureRate float64 `json:"failure_rate"`
	P50Seconds  int64   `json:"p50_seconds"`
	LastSeconds int64   `json:"last_seconds"`
	Flaky       bool    `json:"flaky"`
	FlakyReason string  `json:"flaky_reason,omitempty"`
}

// PipelineJobDelta is a job's duration change between two runs.
type PipelineJobDelta struct {
	Job             string  `json:"job"`
	BaselineSeconds int64   `json:"baseline_seconds"`
	CurrentSeconds  int64   `json:"current_seconds"`
	DeltaSeconds    int64   `json:"delta_seconds"`
	DeltaPercent    float64 `json:"delta_percent"`
}

// PipelineInsights is the run-over-run view of one pipeline.
type PipelineInsights struct {
	Window     int                  `json:"window"`
	Jobs       []PipelineJobInsight `json:"jobs"`
	DiffRunID  string               `json:"diff_run_id,omitempty"`
	DiffBaseID string               `json:"diff_base_run_id,omitempty"`
	Diff       []PipelineJobDelta   `json:"diff"`
}

// ListPipelineRunFixes calls GET .../pipeline-runs/{id}/fixes.
func (c *Client) ListPipelineRunFixes(ctx context.Context, app, runID string) ([]PipelineFix, error) {
	var out []PipelineFix
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipeline-runs", runID, "fixes"), nil, &out)
}

// ApplyPipelineRunFix calls POST .../pipeline-runs/{id}/fixes/apply.
func (c *Client) ApplyPipelineRunFix(ctx context.Context, app, runID, kind, job string, rerun bool) (PipelineFixResult, error) {
	var out PipelineFixResult
	body := map[string]any{"kind": kind, "job": job, "rerun": rerun}
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipeline-runs", runID, "fixes", "apply"), body, &out)
}

// GetPipelineInsights calls GET .../pipelines/{name}/insights.
func (c *Client) GetPipelineInsights(ctx context.Context, app, name string) (PipelineInsights, error) {
	var out PipelineInsights
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipelines", name, "insights"), nil, &out)
}

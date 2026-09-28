package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PipelineHold is a run held for approval, such as a fork pull request.
type PipelineHold struct {
	State  string     `json:"state"`
	Reason string     `json:"reason"`
	By     string     `json:"by,omitempty"`
	At     *time.Time `json:"at,omitempty"`
}

// PipelineSyncStatus is an app's repository sync settings and last outcome.
type PipelineSyncStatus struct {
	Connected    bool       `json:"connected"`
	RepoIsTruth  bool       `json:"repo_is_truth"`
	LastSHA      string     `json:"last_sha,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// PipelineSyncItem is the outcome for one pipeline file.
type PipelineSyncItem struct {
	File    string `json:"file"`
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
}

// PipelineSyncResult is the outcome of one repository sync.
type PipelineSyncResult struct {
	SHA   string             `json:"sha"`
	Dir   string             `json:"dir,omitempty"`
	Items []PipelineSyncItem `json:"items"`
}

// PipelineTrigger is one recorded decision about a git event.
type PipelineTrigger struct {
	ID        int64     `json:"id"`
	Pipeline  string    `json:"pipeline,omitempty"`
	Event     string    `json:"event"`
	Ref       string    `json:"ref,omitempty"`
	SHA       string    `json:"sha,omitempty"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	RunID     string    `json:"run_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// GetPipelineSync calls GET /api/v1/apps/{app}/pipeline-sync.
func (c *Client) GetPipelineSync(ctx context.Context, app string) (PipelineSyncStatus, error) {
	var out PipelineSyncStatus
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipeline-sync"), nil, &out)
}

// SetPipelineSync calls PUT /api/v1/apps/{app}/pipeline-sync.
func (c *Client) SetPipelineSync(ctx context.Context, app string, repoIsTruth bool) (PipelineSyncStatus, error) {
	var out PipelineSyncStatus
	body := map[string]bool{"repo_is_truth": repoIsTruth}
	return out, c.do(ctx, http.MethodPut, pipelinePath(app, "pipeline-sync"), body, &out)
}

// RunPipelineSync calls POST /api/v1/apps/{app}/pipeline-sync.
func (c *Client) RunPipelineSync(ctx context.Context, app string) (PipelineSyncResult, error) {
	var out PipelineSyncResult
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipeline-sync"), nil, &out)
}

// ListPipelineTriggers calls GET /api/v1/apps/{app}/pipeline-triggers.
func (c *Client) ListPipelineTriggers(ctx context.Context, app string) ([]PipelineTrigger, error) {
	var out []PipelineTrigger
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipeline-triggers"), nil, &out)
}

// DecidePipelineRunHold calls POST .../pipeline-runs/{id}/hold.
func (c *Client) DecidePipelineRunHold(ctx context.Context, app, runID, decision string) error {
	return c.do(ctx, http.MethodPost, pipelinePath(app, "pipeline-runs", runID, "hold"), map[string]string{"decision": decision}, nil)
}

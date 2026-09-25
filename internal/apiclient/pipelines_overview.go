package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PipelineRunRow mirrors internal/api's pipelineRunRowResource.
type PipelineRunRow struct {
	ID              string     `json:"id"`
	App             string     `json:"app"`
	Pipeline        string     `json:"pipeline"`
	Number          int        `json:"number"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	Trigger         string     `json:"trigger"`
	Ref             string     `json:"ref,omitempty"`
	ShortSHA        string     `json:"short_sha,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	DurationSeconds *int64     `json:"duration_seconds,omitempty"`
	ApprovalPending bool       `json:"approval_pending"`
	ApprovalID      int64      `json:"approval_id,omitempty"`
	HoldPending     bool       `json:"hold_pending"`
	CanDecide       bool       `json:"can_decide"`
}

// PipelineRunRowsPage is one page of cross-app runs.
type PipelineRunRowsPage struct {
	Runs       []PipelineRunRow `json:"runs"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// PipelineRunsQuery filters ListAllPipelineRuns. Empty fields are not sent.
type PipelineRunsQuery struct {
	Status   string
	App      string
	Pipeline string
	Trigger  string
	Cursor   string
	Limit    int
}

// ListAllPipelineRuns calls GET /api/v1/pipeline-runs.
func (c *Client) ListAllPipelineRuns(ctx context.Context, q PipelineRunsQuery) (PipelineRunRowsPage, error) {
	v := url.Values{}
	for k, val := range map[string]string{"status": q.Status, "app": q.App, "pipeline": q.Pipeline, "trigger": q.Trigger, "cursor": q.Cursor} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/api/v1/pipeline-runs"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out PipelineRunRowsPage
	return out, c.do(ctx, http.MethodGet, path, nil, &out)
}

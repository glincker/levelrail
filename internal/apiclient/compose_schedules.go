package apiclient

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// ComposeSchedule is one member service's schedule, tagged with its compose key.
type ComposeSchedule struct {
	Service string `json:"service"`
	ScheduledTaskResource
}

// ComposeScheduleRun is one recorded run of a compose schedule.
type ComposeScheduleRun struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"task_id"`
	Trigger    string     `json:"trigger"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	DurationMS int64      `json:"duration_ms"`
	Message    string     `json:"message,omitempty"`
	Output     string     `json:"output,omitempty"`
}

// ComposeScheduleStarted is the answer to a run-now request.
type ComposeScheduleStarted struct {
	ID    string `json:"id"`
	RunID string `json:"run_id,omitempty"`
}

func composeSchedulesPath(stack, service string) string {
	return "/api/v1/apps/" + PathEscape(stack) + "/compose/services/" + PathEscape(service) + "/schedules"
}

// ListComposeSchedules calls GET /api/v1/apps/{name}/compose/schedules.
func (c *Client) ListComposeSchedules(ctx context.Context, stack string) ([]ComposeSchedule, error) {
	var out struct {
		Schedules []ComposeSchedule `json:"schedules"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(stack)+"/compose/schedules", nil, &out)
	return out.Schedules, err
}

// CreateComposeSchedule calls POST .../compose/services/{service}/schedules.
func (c *Client) CreateComposeSchedule(ctx context.Context, stack, service string, req ScheduledTaskRequest) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPost, composeSchedulesPath(stack, service), req, &out)
	return out, err
}

// RunComposeSchedule calls POST .../schedules/{id}/run.
func (c *Client) RunComposeSchedule(ctx context.Context, stack, service, id string) (ComposeScheduleStarted, error) {
	var out ComposeScheduleStarted
	err := c.do(ctx, http.MethodPost, composeSchedulesPath(stack, service)+"/"+PathEscape(id)+"/run", nil, &out)
	return out, err
}

// SetComposeScheduleEnabled calls POST .../schedules/{id}/enable or /disable.
func (c *Client) SetComposeScheduleEnabled(ctx context.Context, stack, service, id string, enabled bool) error {
	verb := "/disable"
	if enabled {
		verb = "/enable"
	}
	return c.do(ctx, http.MethodPost, composeSchedulesPath(stack, service)+"/"+PathEscape(id)+verb, nil, nil)
}

// DeleteComposeSchedule calls DELETE .../schedules/{id}.
func (c *Client) DeleteComposeSchedule(ctx context.Context, stack, service, id string) error {
	return c.do(ctx, http.MethodDelete, composeSchedulesPath(stack, service)+"/"+PathEscape(id), nil, nil)
}

// ListComposeScheduleRuns calls GET .../schedules/{id}/runs, newest first.
func (c *Client) ListComposeScheduleRuns(ctx context.Context, stack, service, id string, limit int) ([]ComposeScheduleRun, error) {
	var out []ComposeScheduleRun
	path := composeSchedulesPath(stack, service) + "/" + PathEscape(id) + "/runs"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppRestartSchedule is an app's scheduled restart, matching internal/api's appRestartScheduleResource.
type AppRestartSchedule struct {
	App      string `json:"app"`
	Schedule *struct {
		Schedule   string `json:"schedule"`
		Timezone   string `json:"timezone"`
		SetBy      string `json:"set_by"`
		NextRunAt  string `json:"next_run_at"`
		LastRunAt  string `json:"last_run_at"`
		LastStatus string `json:"last_status"`
		LastReason string `json:"last_reason"`
		Failures   int    `json:"failures"`
	} `json:"schedule,omitempty"`
	MinInterval string `json:"min_interval"`
}

func restartSchedulePath(name string) string {
	return "/api/v1/apps/" + url.PathEscape(name) + "/restart-schedule"
}

// GetAppRestartSchedule reads the app's restart schedule.
func (c *Client) GetAppRestartSchedule(ctx context.Context, name string) (AppRestartSchedule, error) {
	var out AppRestartSchedule
	err := c.do(ctx, http.MethodGet, restartSchedulePath(name), nil, &out)
	return out, err
}

// SetAppRestartSchedule sets a cron or "@every <duration>" restart schedule.
func (c *Client) SetAppRestartSchedule(ctx context.Context, name, schedule, timezone string) (AppRestartSchedule, error) {
	var out AppRestartSchedule
	body := map[string]string{"schedule": schedule, "timezone": timezone}
	err := c.do(ctx, http.MethodPut, restartSchedulePath(name), body, &out)
	return out, err
}

// ClearAppRestartSchedule removes the schedule.
func (c *Client) ClearAppRestartSchedule(ctx context.Context, name string) (AppRestartSchedule, error) {
	var out AppRestartSchedule
	err := c.do(ctx, http.MethodDelete, restartSchedulePath(name), nil, &out)
	return out, err
}

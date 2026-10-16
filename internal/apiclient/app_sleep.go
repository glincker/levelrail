package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AppSleepResource mirrors internal/api's appSleepResource.
type AppSleepResource struct {
	Enabled     bool `json:"enabled"`
	IdleMinutes int  `json:"idle_minutes"`
	Sleeping    bool `json:"sleeping"`
	// Since is when the app went to sleep, set only while it is asleep.
	Since *time.Time `json:"since,omitempty"`
	// HoldRequests is function mode: a request that wakes the app waits for it.
	HoldRequests bool `json:"hold_requests"`
}

func sleepPath(appName string) string { return "/api/v1/apps/" + PathEscape(appName) + "/sleep" }

// GetAppSleep calls GET /api/v1/apps/{name}/sleep.
func (c *Client) GetAppSleep(ctx context.Context, appName string) (AppSleepResource, error) {
	var out AppSleepResource
	err := c.do(ctx, http.MethodGet, sleepPath(appName), nil, &out)
	return out, err
}

// SetAppSleep calls PUT /api/v1/apps/{name}/sleep; 0 minutes turns sleeping off.
func (c *Client) SetAppSleep(ctx context.Context, appName string, idleMinutes int) (AppSleepResource, error) {
	var out AppSleepResource
	err := c.do(ctx, http.MethodPut, sleepPath(appName), map[string]int{"idle_minutes": idleMinutes}, &out)
	return out, err
}

// WakeApp calls POST /api/v1/apps/{name}/sleep/wake.
func (c *Client) WakeApp(ctx context.Context, appName string) (AppSleepResource, error) {
	var out AppSleepResource
	err := c.do(ctx, http.MethodPost, sleepPath(appName)+"/wake", nil, &out)
	return out, err
}

// SetAppSleepHold calls PUT /api/v1/apps/{name}/sleep with function mode on or off.
func (c *Client) SetAppSleepHold(ctx context.Context, appName string, idleMinutes int, hold bool) (AppSleepResource, error) {
	var out AppSleepResource
	err := c.do(ctx, http.MethodPut, sleepPath(appName), map[string]any{"idle_minutes": idleMinutes, "hold_requests": hold}, &out)
	return out, err
}

// SleepFleetApp mirrors internal/api's sleepFleetApp.
type SleepFleetApp struct {
	Name             string     `json:"name"`
	IdleMinutes      int        `json:"idle_minutes"`
	Sleeping         bool       `json:"sleeping"`
	MemoryLimitBytes int64      `json:"memory_limit_bytes"`
	Since            *time.Time `json:"since,omitempty"`
}

// SleepFleet mirrors internal/api's sleepFleetResource.
type SleepFleet struct {
	Enabled        int             `json:"enabled"`
	Sleeping       int             `json:"sleeping"`
	ReclaimedBytes int64           `json:"reclaimed_bytes"`
	Apps           []SleepFleetApp `json:"apps"`
}

// ListSleepFleet calls GET /api/v1/sleep.
func (c *Client) ListSleepFleet(ctx context.Context) (SleepFleet, error) {
	var out SleepFleet
	err := c.do(ctx, http.MethodGet, "/api/v1/sleep", nil, &out)
	return out, err
}

// SleepAppNow calls POST /api/v1/apps/{name}/sleep/now.
func (c *Client) SleepAppNow(ctx context.Context, appName string) (AppSleepResource, error) {
	var out AppSleepResource
	err := c.do(ctx, http.MethodPost, sleepPath(appName)+"/now", nil, &out)
	return out, err
}

// SleepEvent mirrors internal/api's sleepEventResource.
type SleepEvent struct {
	ID              int64     `json:"id"`
	Kind            string    `json:"kind"`
	At              time.Time `json:"at"`
	Reason          string    `json:"reason,omitempty"`
	IdleMinutes     int       `json:"idle_minutes,omitempty"`
	TriggerPath     string    `json:"trigger_path,omitempty"`
	TriggerSource   string    `json:"trigger_source,omitempty"`
	Detail          string    `json:"detail,omitempty"`
	DurationSeconds *int64    `json:"duration_seconds,omitempty"`
	WakeLatencyMs   *int64    `json:"wake_latency_ms,omitempty"`
	Actor           string    `json:"actor"`
}

// SleepStats mirrors internal/api's sleepStatsResource.
type SleepStats struct {
	CurrentlyAsleepSince   *time.Time `json:"currently_asleep_since,omitempty"`
	AsleepSeconds7d        int64      `json:"asleep_seconds_7d"`
	AsleepSeconds30d       int64      `json:"asleep_seconds_30d"`
	WakeCount7d            int        `json:"wake_count_7d"`
	AvgWakeLatencyMs       *int64     `json:"avg_wake_latency_ms,omitempty"`
	P95WakeLatencyMs       *int64     `json:"p95_wake_latency_ms,omitempty"`
	LastSleptAt            *time.Time `json:"last_slept_at,omitempty"`
	LastWokeAt             *time.Time `json:"last_woke_at,omitempty"`
	LastWakeReason         string     `json:"last_wake_reason,omitempty"`
	MemoryGiBHoursSaved30d *float64   `json:"memory_gib_hours_saved_30d,omitempty"`
}

// SleepHistory mirrors internal/api's sleepHistoryResource.
type SleepHistory struct {
	Enabled  bool         `json:"enabled"`
	Sleeping bool         `json:"sleeping"`
	Events   []SleepEvent `json:"events"`
	Stats    SleepStats   `json:"stats"`
}

// GetSleepHistory calls GET /api/v1/apps/{name}/sleep/history. since is a
// duration such as 7d or empty, limit 0 uses the server default.
func (c *Client) GetSleepHistory(ctx context.Context, appName, since string, limit int) (SleepHistory, error) {
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := sleepPath(appName) + "/history"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out SleepHistory
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

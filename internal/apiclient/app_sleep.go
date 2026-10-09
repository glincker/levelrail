package apiclient

import (
	"context"
	"net/http"
)

// AppSleepResource mirrors internal/api's appSleepResource.
type AppSleepResource struct {
	Enabled     bool `json:"enabled"`
	IdleMinutes int  `json:"idle_minutes"`
	Sleeping    bool `json:"sleeping"`
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

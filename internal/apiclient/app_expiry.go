package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppExpiry is the expiry state of one app, matching internal/api's appExpiryResource.
type AppExpiry struct {
	App    string `json:"app"`
	Expiry *struct {
		ExpiresAt  string `json:"expires_at"`
		DeleteData bool   `json:"delete_data"`
		SetBy      string `json:"set_by"`
		StoppedAt  string `json:"stopped_at"`
		KeptReason string `json:"kept_reason"`
	} `json:"expiry,omitempty"`
	State          string `json:"state"`
	DeleteAfter    string `json:"delete_after,omitempty"`
	DeleteEnabled  bool   `json:"delete_enabled"`
	GraceHours     int    `json:"grace_hours"`
	NoticeHours    int    `json:"notice_hours"`
	MaxHorizonDays int    `json:"max_horizon_days"`
}

func expiryPath(name string) string {
	return "/api/v1/apps/" + url.PathEscape(name) + "/expiry"
}

// GetAppExpiry reads the app's expiry.
func (c *Client) GetAppExpiry(ctx context.Context, name string) (AppExpiry, error) {
	var out AppExpiry
	err := c.do(ctx, http.MethodGet, expiryPath(name), nil, &out)
	return out, err
}

// SetAppExpiry schedules the app's expiry; confirm must be the app name when deleteData is set.
func (c *Client) SetAppExpiry(ctx context.Context, name, expiresAt string, deleteData bool, confirm string) (AppExpiry, error) {
	var out AppExpiry
	body := map[string]any{"expires_at": expiresAt, "delete_data": deleteData, "confirm": confirm}
	err := c.do(ctx, http.MethodPut, expiryPath(name), body, &out)
	return out, err
}

// ClearAppExpiry removes the expiry and cancels any pending delete.
func (c *Client) ClearAppExpiry(ctx context.Context, name string) (AppExpiry, error) {
	var out AppExpiry
	err := c.do(ctx, http.MethodDelete, expiryPath(name), nil, &out)
	return out, err
}

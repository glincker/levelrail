package main

import (
	"context"
	"net/http"
	"net/url"
)

// notificationChannelResource mirrors internal/api's
// notificationChannelResource (internal/api/notification_channels.go).
type notificationChannelResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// createNotificationChannelRequest mirrors internal/api's
// createNotificationChannelRequest.
type createNotificationChannelRequest struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

// testNotificationChannelRequest mirrors internal/api's
// testNotificationChannelRequest.
type testNotificationChannelRequest struct {
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
}

// pushoverEndpoint is Pushover's fixed Message API endpoint. A pushover
// notify_url is this URL plus token/user query params, matching
// internal/alerting/notify.go's own parsePushoverCreds convention.
const pushoverEndpoint = "https://api.pushover.net/1/messages.json"

// buildPushoverNotifyURL packs a Pushover user key and API token into
// the notify_url shape the control plane expects, so an operator can
// pass the two Pushover credentials as separate flags instead of
// hand-building a query string.
func buildPushoverNotifyURL(userKey, apiToken string) string {
	q := url.Values{}
	q.Set("token", apiToken)
	q.Set("user", userKey)
	return pushoverEndpoint + "?" + q.Encode()
}

// CreateNotificationChannel calls POST /api/v1/notification-channels.
func (c *Client) CreateNotificationChannel(ctx context.Context, req createNotificationChannelRequest) (notificationChannelResource, error) {
	var out notificationChannelResource
	err := c.do(ctx, http.MethodPost, "/api/v1/notification-channels", req, &out)
	return out, err
}

// ListNotificationChannels calls GET /api/v1/notification-channels.
func (c *Client) ListNotificationChannels(ctx context.Context) ([]notificationChannelResource, error) {
	var out []notificationChannelResource
	err := c.do(ctx, http.MethodGet, "/api/v1/notification-channels", nil, &out)
	return out, err
}

// DeleteNotificationChannel calls DELETE
// /api/v1/notification-channels/{id}.
func (c *Client) DeleteNotificationChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/notification-channels/"+pathEscape(id), nil, nil)
}

// TestNotificationChannel calls POST
// /api/v1/notification-channels/test: fires a real test message via
// kind/notifyURL without requiring the channel to exist yet.
func (c *Client) TestNotificationChannel(ctx context.Context, kind, notifyURL string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/notification-channels/test", testNotificationChannelRequest{Kind: kind, NotifyURL: notifyURL}, nil)
}

// TestExistingNotificationChannel calls POST
// /api/v1/notification-channels/{id}/test: the same real send, against
// an already-saved channel's own kind/notify_url.
func (c *Client) TestExistingNotificationChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/notification-channels/"+pathEscape(id)+"/test", nil, nil)
}

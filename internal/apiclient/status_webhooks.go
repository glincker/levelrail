package apiclient

import (
	"context"
	"net/http"
	"time"
)

// StatusWebhook mirrors internal/api's statusWebhookResource. SigningSecret is set only by create and rotate.
type StatusWebhook struct {
	ID                  string    `json:"id,omitempty"`
	URL                 string    `json:"url"`
	Label               string    `json:"label"`
	Events              []string  `json:"events"`
	Paused              bool      `json:"paused"`
	PausedReason        string    `json:"paused_reason,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	SigningSecret       string    `json:"signing_secret,omitempty"`
}

// StatusWebhookDelivery mirrors internal/api's statusWebhookDeliveryResource.
type StatusWebhookDelivery struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	EventType  string    `json:"event_type"`
	State      string    `json:"state"`
	Attempts   int       `json:"attempts"`
	StatusCode int       `json:"status_code"`
	LastError  string    `json:"last_error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// StatusWebhookTest is the result of a test ping.
type StatusWebhookTest struct {
	Delivered  bool   `json:"delivered"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

const statusWebhooksPath = "/api/v1/status-page/webhooks"

// ListStatusWebhooks calls GET /api/v1/status-page/webhooks.
func (c *Client) ListStatusWebhooks(ctx context.Context) ([]StatusWebhook, error) {
	var out []StatusWebhook
	err := c.do(ctx, http.MethodGet, statusWebhooksPath, nil, &out)
	return out, err
}

// CreateStatusWebhook calls POST /api/v1/status-page/webhooks.
func (c *Client) CreateStatusWebhook(ctx context.Context, w StatusWebhook) (StatusWebhook, error) {
	var out StatusWebhook
	err := c.do(ctx, http.MethodPost, statusWebhooksPath, w, &out)
	return out, err
}

// StatusWebhookAction calls POST /api/v1/status-page/webhooks/{id}/{rotate|pause|resume}.
func (c *Client) StatusWebhookAction(ctx context.Context, id, action string) (StatusWebhook, error) {
	var out StatusWebhook
	err := c.do(ctx, http.MethodPost, statusWebhooksPath+"/"+PathEscape(id)+"/"+action, nil, &out)
	return out, err
}

// TestStatusWebhook calls POST /api/v1/status-page/webhooks/{id}/test.
func (c *Client) TestStatusWebhook(ctx context.Context, id string) (StatusWebhookTest, error) {
	var out StatusWebhookTest
	err := c.do(ctx, http.MethodPost, statusWebhooksPath+"/"+PathEscape(id)+"/test", nil, &out)
	return out, err
}

// DeleteStatusWebhook calls DELETE /api/v1/status-page/webhooks/{id}.
func (c *Client) DeleteStatusWebhook(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, statusWebhooksPath+"/"+PathEscape(id), nil, nil)
}

// ListStatusWebhookDeliveries calls GET /api/v1/status-page/webhooks/{id}/deliveries.
func (c *Client) ListStatusWebhookDeliveries(ctx context.Context, id string) ([]StatusWebhookDelivery, error) {
	var out []StatusWebhookDelivery
	err := c.do(ctx, http.MethodGet, statusWebhooksPath+"/"+PathEscape(id)+"/deliveries", nil, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
)

// EventWebhookResource mirrors GET /api/v1/event-webhooks items. The signing
// secret is set only on create and rotate responses.
type EventWebhookResource struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Enabled       bool     `json:"enabled"`
	Events        []string `json:"events"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Pending       int      `json:"pending"`
	Delivered     int      `json:"delivered"`
	Dead          int      `json:"dead"`
	LastStatus    int      `json:"last_status"`
	SigningSecret string   `json:"signing_secret,omitempty"`
}

// EventDeliveryResource is one entry of a webhook's delivery log.
type EventDeliveryResource struct {
	ID             string `json:"id"`
	WebhookID      string `json:"webhook_id"`
	EventID        string `json:"event_id"`
	EventType      string `json:"event_type"`
	State          string `json:"state"`
	Attempts       int    `json:"attempts"`
	ResponseStatus int    `json:"response_status"`
	LastError      string `json:"last_error,omitempty"`
	NextAttemptAt  string `json:"next_attempt_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// CreateEventWebhookRequest is the POST /api/v1/event-webhooks body.
type CreateEventWebhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Enabled *bool    `json:"enabled,omitempty"`
	Events  []string `json:"events,omitempty"`
}

// UpdateEventWebhookRequest is the PATCH body; nil fields stay unchanged.
type UpdateEventWebhookRequest struct {
	Name    *string   `json:"name,omitempty"`
	URL     *string   `json:"url,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
	Events  *[]string `json:"events,omitempty"`
}

// EventWebhookTestResult is the answer of the test probe.
type EventWebhookTestResult struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func eventWebhookPath(id string) string { return "/api/v1/event-webhooks/" + PathEscape(id) }

// ListEventWebhooks calls GET /api/v1/event-webhooks.
func (c *Client) ListEventWebhooks(ctx context.Context) ([]EventWebhookResource, error) {
	var out []EventWebhookResource
	err := c.do(ctx, http.MethodGet, "/api/v1/event-webhooks", nil, &out)
	return out, err
}

// CreateEventWebhook calls POST /api/v1/event-webhooks.
func (c *Client) CreateEventWebhook(ctx context.Context, req CreateEventWebhookRequest) (EventWebhookResource, error) {
	var out EventWebhookResource
	err := c.do(ctx, http.MethodPost, "/api/v1/event-webhooks", req, &out)
	return out, err
}

// UpdateEventWebhook calls PATCH /api/v1/event-webhooks/{id}.
func (c *Client) UpdateEventWebhook(ctx context.Context, id string, req UpdateEventWebhookRequest) (EventWebhookResource, error) {
	var out EventWebhookResource
	err := c.do(ctx, http.MethodPatch, eventWebhookPath(id), req, &out)
	return out, err
}

// DeleteEventWebhook calls DELETE /api/v1/event-webhooks/{id}.
func (c *Client) DeleteEventWebhook(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, eventWebhookPath(id), nil, nil)
}

// TestEventWebhook calls POST /api/v1/event-webhooks/{id}/test.
func (c *Client) TestEventWebhook(ctx context.Context, id string) (EventWebhookTestResult, error) {
	var out EventWebhookTestResult
	err := c.do(ctx, http.MethodPost, eventWebhookPath(id)+"/test", nil, &out)
	return out, err
}

// RotateEventWebhookSecret calls POST /api/v1/event-webhooks/{id}/rotate-secret.
func (c *Client) RotateEventWebhookSecret(ctx context.Context, id string) (EventWebhookResource, error) {
	var out EventWebhookResource
	err := c.do(ctx, http.MethodPost, eventWebhookPath(id)+"/rotate-secret", nil, &out)
	return out, err
}

// ListEventWebhookDeliveries calls GET /api/v1/event-webhooks/{id}/deliveries.
func (c *Client) ListEventWebhookDeliveries(ctx context.Context, id string) ([]EventDeliveryResource, error) {
	var out []EventDeliveryResource
	err := c.do(ctx, http.MethodGet, eventWebhookPath(id)+"/deliveries", nil, &out)
	return out, err
}

// ReplayEventWebhookDelivery calls POST /api/v1/event-webhook-deliveries/{id}/replay.
func (c *Client) ReplayEventWebhookDelivery(ctx context.Context, id string) (EventDeliveryResource, error) {
	var out EventDeliveryResource
	err := c.do(ctx, http.MethodPost, "/api/v1/event-webhook-deliveries/"+PathEscape(id)+"/replay", nil, &out)
	return out, err
}

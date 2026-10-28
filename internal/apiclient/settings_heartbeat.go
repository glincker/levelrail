package apiclient

import (
	"context"
	"net/http"
	"time"
)

// HeartbeatResource mirrors internal/heartbeat's Status (GET /api/v1/settings/heartbeat). Hosts only, never URLs.
type HeartbeatResource struct {
	Configured   bool      `json:"configured"`
	Source       string    `json:"source,omitempty"`
	Host         string    `json:"host,omitempty"`
	FailHost     string    `json:"fail_host,omitempty"`
	IntervalSecs int       `json:"interval_seconds"`
	BodyEnabled  bool      `json:"body_enabled"`
	CanStoreURL  bool      `json:"can_store_url"`
	LastPingAt   time.Time `json:"last_ping_at,omitzero"`
	LastResult   string    `json:"last_result,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

// HeartbeatUpdate is the PUT body; nil fields are left unchanged and an empty URL clears it.
type HeartbeatUpdate struct {
	URL     *string `json:"url,omitempty"`
	FailURL *string `json:"fail_url,omitempty"`
	Body    *bool   `json:"body_enabled,omitempty"`
}

// GetHeartbeat calls GET /api/v1/settings/heartbeat.
func (c *Client) GetHeartbeat(ctx context.Context) (HeartbeatResource, error) {
	var out HeartbeatResource
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/heartbeat", nil, &out)
	return out, err
}

// UpdateHeartbeat calls PUT /api/v1/settings/heartbeat.
func (c *Client) UpdateHeartbeat(ctx context.Context, req HeartbeatUpdate) (HeartbeatResource, error) {
	var out HeartbeatResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/heartbeat", req, &out)
	return out, err
}

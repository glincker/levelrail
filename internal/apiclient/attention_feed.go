package apiclient

import (
	"context"
	"net/http"
	"time"
)

// Device login states as GET /api/v1/auth/device/activity reports them.
const (
	DeviceLoginWaiting  = "waiting"
	DeviceLoginExpired  = "expired"
	DeviceLoginDenied   = "denied"
	DeviceLoginApproved = "approved"
)

// DeviceActivity is one recent CLI login in any state. It never carries a
// user code or device code.
type DeviceActivity struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	ClientName  string    `json:"client_name"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Dismissible bool      `json:"dismissible"`
	Dismissed   bool      `json:"dismissed"`
	AuditPath   string    `json:"audit_path"`
}

// ListDeviceActivity calls GET /api/v1/auth/device/activity.
func (c *Client) ListDeviceActivity(ctx context.Context) ([]DeviceActivity, error) {
	var out struct {
		Items []DeviceActivity `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/device/activity", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// AttentionFeedItem is one server-built attention item (GET
// /api/v1/attention/feed), already limited to what the caller may see.
type AttentionFeedItem struct {
	ID       string            `json:"id"`
	Severity string            `json:"severity"`
	Kind     string            `json:"kind"`
	Subject  string            `json:"subject"`
	Title    string            `json:"title"`
	Detail   string            `json:"detail"`
	Action   string            `json:"action"`
	Link     string            `json:"link"`
	Params   map[string]string `json:"params,omitempty"`
}

// GetAttentionFeed calls GET /api/v1/attention/feed.
func (c *Client) GetAttentionFeed(ctx context.Context) ([]AttentionFeedItem, error) {
	var out struct {
		Items []AttentionFeedItem `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/attention/feed", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

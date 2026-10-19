package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// InboxItem mirrors internal/api's inboxItemResource.
type InboxItem struct {
	AlertHistoryEntry
	Read bool `json:"read"`
}

// InboxPrefs mirrors internal/api's inboxPrefsResource.
type InboxPrefs struct {
	MinSeverity     string     `json:"min_severity"`
	MutedApps       []string   `json:"muted_apps"`
	DigestEnabled   bool       `json:"digest_enabled"`
	DigestHour      int        `json:"digest_hour"`
	DigestChannelID string     `json:"digest_channel_id"`
	LastDigestAt    *time.Time `json:"last_digest_at,omitempty"`
}

// ListInbox calls GET /api/v1/inbox.
func (c *Client) ListInbox(ctx context.Context, unreadOnly bool, limit int) ([]InboxItem, error) {
	v := url.Values{}
	if unreadOnly {
		v.Set("unread", "true")
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/inbox"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out []InboxItem
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// InboxUnreadCount calls GET /api/v1/inbox/unread-count.
func (c *Client) InboxUnreadCount(ctx context.Context) (int, error) {
	var out struct {
		Unread int `json:"unread"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/inbox/unread-count", nil, &out)
	return out.Unread, err
}

// MarkInboxRead calls POST /api/v1/inbox/read with ids, or with all=true.
func (c *Client) MarkInboxRead(ctx context.Context, ids []string, all bool) error {
	body := map[string]any{"ids": ids, "all": all}
	return c.do(ctx, http.MethodPost, "/api/v1/inbox/read", body, nil)
}

// GetInboxPrefs calls GET /api/v1/inbox/preferences.
func (c *Client) GetInboxPrefs(ctx context.Context) (InboxPrefs, error) {
	var out InboxPrefs
	err := c.do(ctx, http.MethodGet, "/api/v1/inbox/preferences", nil, &out)
	return out, err
}

// SetInboxPrefs calls PUT /api/v1/inbox/preferences.
func (c *Client) SetInboxPrefs(ctx context.Context, p InboxPrefs) (InboxPrefs, error) {
	var out InboxPrefs
	err := c.do(ctx, http.MethodPut, "/api/v1/inbox/preferences", p, &out)
	return out, err
}

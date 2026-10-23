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

// InboxState mirrors internal/api's inboxStateResource.
type InboxState struct {
	ReadIDs     []string `json:"read_ids"`
	ArchivedIDs []string `json:"archived_ids"`
}

// GetInboxState calls GET /api/v1/inbox/state.
func (c *Client) GetInboxState(ctx context.Context) (InboxState, error) {
	var out InboxState
	err := c.do(ctx, http.MethodGet, "/api/v1/inbox/state", nil, &out)
	return out, err
}

// ArchiveInbox calls POST /api/v1/inbox/archive; restore undoes an archive.
func (c *Client) ArchiveInbox(ctx context.Context, ids []string, restore bool) error {
	body := map[string]any{"ids": ids, "restore": restore}
	return c.do(ctx, http.MethodPost, "/api/v1/inbox/archive", body, nil)
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

// InboxClearResult mirrors internal/api's inboxClearResource.
type InboxClearResult struct {
	Count  int  `json:"count"`
	DryRun bool `json:"dry_run"`
}

// InboxClearRequest selects items for POST /api/v1/inbox/clear.
type InboxClearRequest struct {
	Action           string `json:"action"`
	Scope            string `json:"scope"`
	OlderThanSeconds int64  `json:"older_than_seconds,omitempty"`
	DryRun           bool   `json:"dry_run,omitempty"`
	ConfirmCount     int    `json:"confirm_count,omitempty"`
}

// DeleteInbox calls POST /api/v1/inbox/delete; restore undoes a delete.
func (c *Client) DeleteInbox(ctx context.Context, ids []string, restore bool) error {
	body := map[string]any{"ids": ids, "restore": restore}
	return c.do(ctx, http.MethodPost, "/api/v1/inbox/delete", body, nil)
}

// ClearInbox calls POST /api/v1/inbox/clear.
func (c *Client) ClearInbox(ctx context.Context, req InboxClearRequest) (InboxClearResult, error) {
	var out InboxClearResult
	err := c.do(ctx, http.MethodPost, "/api/v1/inbox/clear", req, &out)
	return out, err
}

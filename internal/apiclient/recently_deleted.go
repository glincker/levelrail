package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// RecentlyDeleted mirrors internal/api's recentlyDeletedResource.
type RecentlyDeleted struct {
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	DeletedAt  time.Time       `json:"deleted_at"`
	ExpiresAt  time.Time       `json:"expires_at"`
	DeletedBy  string          `json:"deleted_by,omitempty"`
	LastBackup json.RawMessage `json:"last_backup,omitempty"`
}

// ListRecentlyDeleted calls GET /api/v1/recently-deleted.
func (c *Client) ListRecentlyDeleted(ctx context.Context) ([]RecentlyDeleted, error) {
	return getJSON[[]RecentlyDeleted](ctx, c, "/api/v1/recently-deleted")
}

// RestoreRecentlyDeleted calls POST /api/v1/recently-deleted/{kind}/{name}/restore; apps need confirm set to the name.
func (c *Client) RestoreRecentlyDeleted(ctx context.Context, kind, name string) error {
	_, err := c.RestoreRecentlyDeletedResult(ctx, kind, name)
	return err
}

// RestoreAttention is one category a restore could not bring back and the command that recreates it.
type RestoreAttention struct {
	Category string `json:"category"`
	Fix      string `json:"fix"`
}

// RestoreResult is the restore response; NeedsAttention is only set for apps.
type RestoreResult struct {
	Status         string             `json:"status"`
	NeedsAttention []RestoreAttention `json:"needs_attention,omitempty"`
}

// RestoreRecentlyDeletedResult is RestoreRecentlyDeleted that also returns what needs attention.
func (c *Client) RestoreRecentlyDeletedResult(ctx context.Context, kind, name string) (RestoreResult, error) {
	var out RestoreResult
	err := c.do(ctx, http.MethodPost, "/api/v1/recently-deleted/"+PathEscape(kind)+"/"+PathEscape(name)+"/restore", map[string]string{"confirm": name}, &out)
	return out, err
}

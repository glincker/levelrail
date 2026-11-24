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
	return c.do(ctx, http.MethodPost, "/api/v1/recently-deleted/"+PathEscape(kind)+"/"+PathEscape(name)+"/restore", map[string]string{"confirm": name}, nil)
}

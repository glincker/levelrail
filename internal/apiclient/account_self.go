package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MyActivityEntry mirrors internal/api's myActivityEntry.
type MyActivityEntry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Event      string    `json:"event"`
	ActorType  string    `json:"actor_type"`
	TokenName  string    `json:"token_name,omitempty"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	RemoteAddr string    `json:"remote_addr"`
	ClientKind string    `json:"client_kind"`
}

// MyActivity mirrors internal/api's myActivityResponse.
type MyActivity struct {
	Entries    []MyActivityEntry `json:"entries"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// MyActivityOptions are the optional filters of GET /api/v1/auth/my-activity.
type MyActivityOptions struct {
	Limit  int
	Kind   string
	Cursor string
}

// MyActivity calls GET /api/v1/auth/my-activity: only the caller's own rows.
func (c *Client) MyActivity(ctx context.Context, opts MyActivityOptions) (MyActivity, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Kind != "" {
		q.Set("kind", opts.Kind)
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	path := "/api/v1/auth/my-activity"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out MyActivity
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// MyCredentials mirrors internal/api's myCredentialsResponse: live sessions
// and active tokens, never a token secret.
type MyCredentials struct {
	Sessions []Session      `json:"sessions"`
	Tokens   []SessionToken `json:"tokens"`
}

// MyCredentials calls GET /api/v1/auth/my-credentials.
func (c *Client) MyCredentials(ctx context.Context) (MyCredentials, error) {
	var out MyCredentials
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/my-credentials", nil, &out)
	return out, err
}

// Preferences mirrors internal/api's preferencesResource.
type Preferences struct {
	Timezone    string `json:"timezone"`
	Language    string `json:"language"`
	Theme       string `json:"theme"`
	LandingPage string `json:"landing_page"`
	Gravatar    bool   `json:"gravatar"`
}

// GetPreferences calls GET /api/v1/auth/preferences.
func (c *Client) GetPreferences(ctx context.Context) (Preferences, error) {
	var out Preferences
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/preferences", nil, &out)
	return out, err
}

// PutPreferences calls PUT /api/v1/auth/preferences, replacing every field.
func (c *Client) PutPreferences(ctx context.Context, p Preferences) (Preferences, error) {
	var out Preferences
	err := c.do(ctx, http.MethodPut, "/api/v1/auth/preferences", p, &out)
	return out, err
}

// AccountExport mirrors internal/api's accountExportResource.
type AccountExport struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	SizeBytes int       `json:"size_bytes,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StartAccountExport calls POST /api/v1/auth/export.
func (c *Client) StartAccountExport(ctx context.Context) (AccountExport, error) {
	var out AccountExport
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/export", struct{}{}, &out)
	return out, err
}

// AccountExportStatus calls GET /api/v1/auth/export/{id}.
func (c *Client) AccountExportStatus(ctx context.Context, id string) (AccountExport, error) {
	var out AccountExport
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/export/"+PathEscape(id), nil, &out)
	return out, err
}

// DownloadAccountExport calls GET /api/v1/auth/export/{id}/download and returns the zip.
func (c *Client) DownloadAccountExport(ctx context.Context, id string) ([]byte, error) {
	return c.downloadRaw(ctx, "/api/v1/auth/export/"+PathEscape(id)+"/download")
}

// UnlinkProvider calls DELETE /api/v1/auth/link/{provider}.
func (c *Client) UnlinkProvider(ctx context.Context, provider string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/auth/link/"+PathEscape(provider), nil, nil)
}

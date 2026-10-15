package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DBVersionInfo is one selectable database version.
type DBVersionInfo struct {
	Version     string `json:"version"`
	Channel     string `json:"channel"`
	Released    string `json:"released,omitempty"`
	EOL         string `json:"eol,omitempty"`
	Recommended bool   `json:"recommended"`
}

// DBVersionSource links to where a version list came from.
type DBVersionSource struct {
	EOLURL          string `json:"eol_url,omitempty"`
	TagsURL         string `json:"tags_url,omitempty"`
	ReleaseNotesURL string `json:"release_notes_url,omitempty"`
}

// DBVersions is GET /api/v1/database-versions?engine=.
type DBVersions struct {
	Engine      string          `json:"engine"`
	Origin      string          `json:"origin"`
	Versions    []DBVersionInfo `json:"versions"`
	Source      DBVersionSource `json:"source"`
	GeneratedAt string          `json:"generated_at,omitempty"`
	FetchedAt   string          `json:"fetched_at,omitempty"`
	Freshness   string          `json:"freshness"`
	Message     string          `json:"message,omitempty"`
	Cooldown    bool            `json:"cooldown,omitempty"`
}

// DBCatalogStatus is GET /api/v1/database-catalog.
type DBCatalogStatus struct {
	Source              string            `json:"source"`
	Mode                string            `json:"mode"`
	RemoteEnabled       bool              `json:"remote_enabled"`
	DisabledReason      string            `json:"disabled_reason,omitempty"`
	URL                 string            `json:"url"`
	GeneratedAt         string            `json:"generated_at,omitempty"`
	FetchedAt           string            `json:"fetched_at,omitempty"`
	LastChecked         string            `json:"last_checked,omitempty"`
	NextAttempt         string            `json:"next_attempt,omitempty"`
	LastError           string            `json:"last_error,omitempty"`
	Freshness           string            `json:"freshness"`
	ServingStaleCopy    bool              `json:"serving_stale_copy"`
	EmbeddedUpdated     string            `json:"embedded_updated"`
	LiveChecks          map[string]string `json:"live_checks,omitempty"`
	PublicKeyConfigured bool              `json:"public_key_configured"`
}

// DatabaseVersions calls GET /api/v1/database-versions?engine=.
func (c *Client) DatabaseVersions(ctx context.Context, engine string) (DBVersions, error) {
	var out DBVersions
	err := c.do(ctx, http.MethodGet, "/api/v1/database-versions?engine="+url.QueryEscape(engine), nil, &out)
	return out, err
}

// RefreshDatabaseVersions calls POST /api/v1/database-versions/refresh?engine=.
func (c *Client) RefreshDatabaseVersions(ctx context.Context, engine string) (DBVersions, error) {
	var out DBVersions
	err := c.do(ctx, http.MethodPost, "/api/v1/database-versions/refresh?engine="+url.QueryEscape(engine), nil, &out)
	return out, err
}

// DatabaseCatalogStatus calls GET /api/v1/database-catalog.
func (c *Client) DatabaseCatalogStatus(ctx context.Context) (DBCatalogStatus, error) {
	var out DBCatalogStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/database-catalog", nil, &out)
	return out, err
}

// RefreshDatabaseCatalog calls POST /api/v1/database-catalog/refresh.
func (c *Client) RefreshDatabaseCatalog(ctx context.Context) (DBCatalogStatus, error) {
	var out DBCatalogStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/database-catalog/refresh", nil, &out)
	return out, err
}

// SetDatabaseCatalogRemote calls PUT /api/v1/settings/database-catalog.
func (c *Client) SetDatabaseCatalogRemote(ctx context.Context, enabled bool) (DBCatalogStatus, error) {
	var out DBCatalogStatus
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/database-catalog", map[string]bool{"remote_enabled": enabled}, &out)
	return out, err
}

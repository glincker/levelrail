package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// BuildCacheSetting points an app's (or the global) BuildKit remote cache at a
// storage destination. It never carries credentials.
type BuildCacheSetting struct {
	AppName       string `json:"app_name"`
	TargetID      string `json:"target_id"`
	Enabled       bool   `json:"enabled"`
	Mode          string `json:"mode"`
	KeyPrefix     string `json:"key_prefix,omitempty"`
	LastBuildAt   string `json:"last_build_at,omitempty"`
	LastResult    string `json:"last_result,omitempty"`
	LastWarning   string `json:"last_warning,omitempty"`
	LastClearedAt string `json:"last_cleared_at,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

// BuildCacheRequest sets a build cache setting. An empty AppName is the global default.
type BuildCacheRequest struct {
	AppName  string `json:"app_name"`
	TargetID string `json:"target_id"`
	Mode     string `json:"mode,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

// BuildCacheStats summarises the objects under an app's cache prefix.
type BuildCacheStats struct {
	Prefix       string    `json:"prefix"`
	Objects      int       `json:"objects"`
	Bytes        int64     `json:"bytes"`
	LastModified time.Time `json:"last_modified"`
	Truncated    bool      `json:"truncated"`
}

// BuildCacheClearResult reports a cache clear. More means run it again.
type BuildCacheClearResult struct {
	Deleted int  `json:"deleted"`
	More    bool `json:"more"`
}

// ListBuildCache calls GET /api/v1/build-cache.
func (c *Client) ListBuildCache(ctx context.Context) ([]BuildCacheSetting, error) {
	var out []BuildCacheSetting
	err := c.do(ctx, http.MethodGet, "/api/v1/build-cache", nil, &out)
	return out, err
}

// SetBuildCache calls PUT /api/v1/build-cache.
func (c *Client) SetBuildCache(ctx context.Context, req BuildCacheRequest) (BuildCacheSetting, error) {
	var out BuildCacheSetting
	err := c.do(ctx, http.MethodPut, "/api/v1/build-cache", req, &out)
	return out, err
}

// DeleteBuildCache calls DELETE /api/v1/build-cache. An empty app is the global default.
func (c *Client) DeleteBuildCache(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/build-cache?app="+url.QueryEscape(app), nil, nil)
}

// GetBuildCacheStats calls GET /api/v1/build-cache/stats.
func (c *Client) GetBuildCacheStats(ctx context.Context, app string) (BuildCacheStats, error) {
	var out BuildCacheStats
	err := c.do(ctx, http.MethodGet, "/api/v1/build-cache/stats?app="+url.QueryEscape(app), nil, &out)
	return out, err
}

// ClearBuildCache calls POST /api/v1/build-cache/clear.
func (c *Client) ClearBuildCache(ctx context.Context, app string) (BuildCacheClearResult, error) {
	var out BuildCacheClearResult
	err := c.do(ctx, http.MethodPost, "/api/v1/build-cache/clear", map[string]string{"app_name": app}, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// AppBuildCache is one app's BuildKit cache mounts on the control plane builder.
type AppBuildCache struct {
	CacheID     string     `json:"cache_id"`
	SizeBytes   int64      `json:"size_bytes"`
	RecordCount int        `json:"record_count"`
	InUse       bool       `json:"in_use"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// AppBuildCacheClearResult is what clearing an app's cache removed.
type AppBuildCacheClearResult struct {
	CacheID      string `json:"cache_id"`
	FreedBytes   int64  `json:"freed_bytes"`
	RemovedCount int    `json:"removed_count"`
}

// GetAppBuildCache calls GET /api/v1/apps/{name}/build-cache.
func (c *Client) GetAppBuildCache(ctx context.Context, app string) (AppBuildCache, error) {
	var out AppBuildCache
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+url.PathEscape(app)+"/build-cache", nil, &out)
	return out, err
}

// ClearAppBuildCache calls DELETE /api/v1/apps/{name}/build-cache.
func (c *Client) ClearAppBuildCache(ctx context.Context, app string) (AppBuildCacheClearResult, error) {
	var out AppBuildCacheClearResult
	err := c.do(ctx, http.MethodDelete, "/api/v1/apps/"+url.PathEscape(app)+"/build-cache", nil, &out)
	return out, err
}

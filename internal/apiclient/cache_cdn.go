package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// CacheNumbers are hit, miss, stale and bypass counters plus bytes.
type CacheNumbers struct {
	Hits        uint64  `json:"hits"`
	Misses      uint64  `json:"misses"`
	Stale       uint64  `json:"stale"`
	Bypasses    uint64  `json:"bypasses"`
	HitRate     float64 `json:"hit_rate"`
	HitBytes    uint64  `json:"hit_bytes"`
	OriginBytes uint64  `json:"origin_bytes"`
	Entries     int     `json:"entries"`
	Bytes       int64   `json:"bytes"`
}

// CacheLimits are the instance caps and where entries live.
type CacheLimits struct {
	MaxBytes       int64  `json:"max_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	MaxObjectBytes int64  `json:"max_object_bytes"`
	TagHeader      string `json:"tag_header"`
	Location       string `json:"location"`
}

// AppCacheDomain is one domain's cache state under an app.
type AppCacheDomain struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
	Rules   int    `json:"rules"`
	CacheNumbers
}

// AppCacheResource is GET /api/v1/apps/{name}/cache.
type AppCacheResource struct {
	App     string           `json:"app"`
	Limits  CacheLimits      `json:"limits"`
	Totals  CacheNumbers     `json:"totals"`
	Series  []CacheBucket    `json:"series"`
	Domains []AppCacheDomain `json:"domains"`
}

// SystemCacheApp is one app's row of the instance cache table.
type SystemCacheApp struct {
	App            string `json:"app"`
	Domains        int    `json:"domains"`
	EnabledDomains int    `json:"enabled_domains"`
	Requests       uint64 `json:"requests"`
	CacheNumbers
}

// SystemCacheResource is GET /api/v1/system/cache.
type SystemCacheResource struct {
	Limits CacheLimits      `json:"limits"`
	Totals CacheNumbers     `json:"totals"`
	Series []CacheBucket    `json:"series"`
	Apps   []SystemCacheApp `json:"apps"`
}

// AppCachePurgeRequest purges url, prefix, tag or all; Domain narrows it
// to one of the app's domains.
type AppCachePurgeRequest struct {
	Scope  string `json:"scope"`
	Value  string `json:"value,omitempty"`
	Domain string `json:"domain,omitempty"`
}

// AppCachePurgeResource reports what a purge removed per domain.
type AppCachePurgeResource struct {
	App     string `json:"app"`
	Scope   string `json:"scope"`
	Value   string `json:"value,omitempty"`
	Removed int    `json:"removed"`
	Domains []struct {
		Domain  string `json:"domain"`
		Removed int    `json:"removed"`
	} `json:"domains"`
}

// CachePurgeRecord is one row of the purge history.
type CachePurgeRecord struct {
	Domain  string `json:"domain"`
	Scope   string `json:"scope"`
	Value   string `json:"value,omitempty"`
	Removed int    `json:"removed"`
	Actor   string `json:"actor"`
	At      string `json:"at"`
}

// CachePurgesResource is GET /api/v1/apps/{name}/cache/purges.
type CachePurgesResource struct {
	App    string             `json:"app"`
	Purges []CachePurgeRecord `json:"purges"`
}

// CachePresetResource reports which domains the preset changed.
type CachePresetResource struct {
	App     string `json:"app"`
	Domains []struct {
		Domain  string `json:"domain"`
		Applied bool   `json:"applied"`
		Rules   int    `json:"rules"`
	} `json:"domains"`
}

func appCachePath(name, tail string) string {
	return "/api/v1/apps/" + url.PathEscape(name) + "/cache" + tail
}

// GetAppCache calls GET /api/v1/apps/{name}/cache.
func (c *Client) GetAppCache(ctx context.Context, name string) (AppCacheResource, error) {
	var out AppCacheResource
	err := c.do(ctx, http.MethodGet, appCachePath(name, ""), nil, &out)
	return out, err
}

// PurgeAppCache calls POST /api/v1/apps/{name}/cache/purge.
func (c *Client) PurgeAppCache(ctx context.Context, name string, req AppCachePurgeRequest) (AppCachePurgeResource, error) {
	var out AppCachePurgeResource
	err := c.do(ctx, http.MethodPost, appCachePath(name, "/purge"), req, &out)
	return out, err
}

// ListAppCachePurges calls GET /api/v1/apps/{name}/cache/purges.
func (c *Client) ListAppCachePurges(ctx context.Context, name string) (CachePurgesResource, error) {
	var out CachePurgesResource
	err := c.do(ctx, http.MethodGet, appCachePath(name, "/purges"), nil, &out)
	return out, err
}

// ApplyAppCachePreset calls POST /api/v1/apps/{name}/cache/preset; domain
// may be empty for every domain of the app.
func (c *Client) ApplyAppCachePreset(ctx context.Context, name, domain string) (CachePresetResource, error) {
	var out CachePresetResource
	body := struct {
		Domain string `json:"domain,omitempty"`
	}{Domain: domain}
	err := c.do(ctx, http.MethodPost, appCachePath(name, "/preset"), body, &out)
	return out, err
}

// GetSystemCache calls GET /api/v1/system/cache.
func (c *Client) GetSystemCache(ctx context.Context) (SystemCacheResource, error) {
	var out SystemCacheResource
	err := c.do(ctx, http.MethodGet, "/api/v1/system/cache", nil, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// QuotaLimits mirrors internal/api's quotaLimitsResource. 0 is unlimited.
type QuotaLimits struct {
	MaxApps      int64   `json:"max_apps"`
	MaxDatabases int64   `json:"max_databases"`
	MaxMemoryMB  int64   `json:"max_memory_mb"`
	MaxCPU       float64 `json:"max_cpu"`
	MaxPreviews  int64   `json:"max_previews"`
}

// QuotaUsage mirrors internal/api's quotaUsageResource.
type QuotaUsage struct {
	Apps      int64   `json:"apps"`
	Databases int64   `json:"databases"`
	MemoryMB  int64   `json:"memory_mb"`
	CPU       float64 `json:"cpu"`
	Previews  int64   `json:"previews"`
}

// QuotaResource mirrors internal/api's quotaResource.
type QuotaResource struct {
	Scope       string      `json:"scope"`
	ScopeID     string      `json:"scope_id"`
	Name        string      `json:"name"`
	Limits      QuotaLimits `json:"limits"`
	Effective   QuotaLimits `json:"effective_limits"`
	Usage       QuotaUsage  `json:"usage"`
	WarnPercent int         `json:"warn_percent"`
	CanManage   bool        `json:"can_manage"`
}

func quotaPath(scope, id string) string {
	return "/api/v1/" + scope + "s/" + url.PathEscape(id) + "/quota"
}

// GetQuota calls GET /api/v1/{teams|projects}/{id}/quota.
func (c *Client) GetQuota(ctx context.Context, scope, id string) (QuotaResource, error) {
	var out QuotaResource
	err := c.do(ctx, http.MethodGet, quotaPath(scope, id), nil, &out)
	return out, err
}

// SetQuota calls PUT /api/v1/{teams|projects}/{id}/quota.
func (c *Client) SetQuota(ctx context.Context, scope, id string, l QuotaLimits) (QuotaResource, error) {
	var out QuotaResource
	err := c.do(ctx, http.MethodPut, quotaPath(scope, id), l, &out)
	return out, err
}

// ClearQuota calls DELETE /api/v1/{teams|projects}/{id}/quota.
func (c *Client) ClearQuota(ctx context.Context, scope, id string) (QuotaResource, error) {
	var out QuotaResource
	err := c.do(ctx, http.MethodDelete, quotaPath(scope, id), nil, &out)
	return out, err
}

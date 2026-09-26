package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PreviewRecord is one deployment's preview outcome.
type PreviewRecord struct {
	DeploymentID string    `json:"deployment_id"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	HTTPStatus   int       `json:"http_status,omitempty"`
	Path         string    `json:"path"`
	Bytes        int64     `json:"bytes"`
	CapturedAt   time.Time `json:"captured_at"`
	ImageURL     string    `json:"image_url,omitempty"`
}

// PreviewStatus is GET /api/v1/apps/{name}/preview's body.
type PreviewStatus struct {
	App           string `json:"app"`
	Enabled       bool   `json:"enabled"`
	Path          string `json:"path"`
	WaitMS        int    `json:"wait_ms"`
	ServerEnabled bool   `json:"server_enabled"`
	Capturing     bool   `json:"capturing"`
	Image         string `json:"image"`
	BrowserImage  *struct {
		Ref        string     `json:"ref"`
		LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	} `json:"browser_image,omitempty"`
	KeepPerApp int   `json:"keep_per_app"`
	TTLDays    int   `json:"ttl_days"`
	MaxTotalMB int64 `json:"max_total_mb"`
	Storage    struct {
		AppBytes   int64 `json:"app_bytes"`
		AppCount   int   `json:"app_count"`
		TotalBytes int64 `json:"total_bytes"`
		TotalCount int   `json:"total_count"`
	} `json:"storage"`
	Latest *PreviewRecord `json:"latest,omitempty"`
}

// PreviewSettingsRequest is PUT /api/v1/apps/{name}/preview's body; nil
// fields are left unchanged.
type PreviewSettingsRequest struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Path    *string `json:"path,omitempty"`
	WaitMS  *int    `json:"wait_ms,omitempty"`
}

// PreviewPruneResult is POST /api/v1/apps/{name}/preview/prune's body.
type PreviewPruneResult struct {
	Removed    int   `json:"removed"`
	FreedBytes int64 `json:"freed_bytes"`
}

// GetPreview calls GET /api/v1/apps/{name}/preview.
func (c *Client) GetPreview(ctx context.Context, name string) (PreviewStatus, error) {
	var out PreviewStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/preview", nil, &out)
	return out, err
}

// SetPreview calls PUT /api/v1/apps/{name}/preview.
func (c *Client) SetPreview(ctx context.Context, name string, req PreviewSettingsRequest) (PreviewStatus, error) {
	var out PreviewStatus
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/preview", req, &out)
	return out, err
}

// CapturePreview calls POST /api/v1/apps/{name}/preview/capture.
func (c *Client) CapturePreview(ctx context.Context, name string) (PreviewStatus, error) {
	var out PreviewStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/preview/capture", nil, &out)
	return out, err
}

// PrunePreview calls POST /api/v1/apps/{name}/preview/prune.
func (c *Client) PrunePreview(ctx context.Context, name string, all bool) (PreviewPruneResult, error) {
	var out PreviewPruneResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/preview/prune", map[string]bool{"all": all}, &out)
	return out, err
}

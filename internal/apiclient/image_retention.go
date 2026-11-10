package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ImageRetentionResource is one app's image keep count and where it comes from.
type ImageRetentionResource struct {
	App                  string `json:"app"`
	Keep                 int    `json:"keep"`
	EffectiveKeep        int    `json:"effective_keep"`
	Source               string `json:"source"`
	GlobalKeep           int    `json:"global_keep"`
	MinKeep              int    `json:"min_keep"`
	ProtectedEnvironment string `json:"protected_environment,omitempty"`
}

// ImageRetentionRequest sets or previews an app's keep count.
type ImageRetentionRequest struct {
	Keep *int   `json:"keep,omitempty"`
	Node string `json:"node,omitempty"`
}

// ImageRetentionCandidate is one image the cleanup would remove.
type ImageRetentionCandidate struct {
	ID           string    `json:"id"`
	Tags         []string  `json:"tags"`
	SizeBytes    int64     `json:"size_bytes"`
	ReclaimBytes int64     `json:"reclaim_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

// ImageRetentionKept is one image of the app that stays, with the reason.
type ImageRetentionKept struct {
	ID     string   `json:"id"`
	Tags   []string `json:"tags"`
	Reason string   `json:"reason"`
}

// ImageRetentionPreview is the dry run of an app's keep count.
type ImageRetentionPreview struct {
	App           string                    `json:"app"`
	Keep          int                       `json:"keep"`
	EffectiveKeep int                       `json:"effective_keep"`
	DryRun        bool                      `json:"dry_run"`
	Remove        []ImageRetentionCandidate `json:"remove"`
	RemoveCount   int                       `json:"remove_count"`
	ReclaimBytes  int64                     `json:"reclaim_bytes"`
	Kept          []ImageRetentionKept      `json:"kept"`
}

func imageRetentionPath(app string) string {
	return "/api/v1/apps/" + url.PathEscape(app) + "/image-retention"
}

// GetImageRetention calls GET /api/v1/apps/{name}/image-retention.
func (c *Client) GetImageRetention(ctx context.Context, app string) (ImageRetentionResource, error) {
	var out ImageRetentionResource
	err := c.do(ctx, http.MethodGet, imageRetentionPath(app), nil, &out)
	return out, err
}

// SetImageRetention calls PUT /api/v1/apps/{name}/image-retention.
func (c *Client) SetImageRetention(ctx context.Context, app string, keep int) (ImageRetentionResource, error) {
	var out ImageRetentionResource
	err := c.do(ctx, http.MethodPut, imageRetentionPath(app), ImageRetentionRequest{Keep: &keep}, &out)
	return out, err
}

// PreviewImageRetention calls POST /api/v1/apps/{name}/image-retention/preview.
func (c *Client) PreviewImageRetention(ctx context.Context, app string, req ImageRetentionRequest) (ImageRetentionPreview, error) {
	var out ImageRetentionPreview
	err := c.do(ctx, http.MethodPost, imageRetentionPath(app)+"/preview", req, &out)
	return out, err
}

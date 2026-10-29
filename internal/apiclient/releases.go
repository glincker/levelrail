package apiclient

import (
	"context"
	"net/http"
)

// ReleaseResource is one release in GET /api/v1/apps/{name}/releases.
type ReleaseResource struct {
	ID              string `json:"id"`
	ImageRef        string `json:"image_ref"`
	Digest          string `json:"digest"`
	ShortDigest     string `json:"short_digest"`
	SourceAttemptID string `json:"source_attempt_id"`
	CreatedAt       string `json:"created_at"`
	SourceReason    string `json:"source_reason"`
}

// AppReleasesResource is the list plus the app's current binding.
type AppReleasesResource struct {
	Releases         []ReleaseResource `json:"releases"`
	CurrentReleaseID string            `json:"current_release_id"`
}

// ListAppReleases calls GET /api/v1/apps/{name}/releases.
func (c *Client) ListAppReleases(ctx context.Context, name string) (AppReleasesResource, error) {
	var out AppReleasesResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/releases", nil, &out)
	return out, err
}

package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// FailedDeployResource is one entry of GET /api/v1/deploys/failed: an app's
// latest failed deploy attempt plus the image of its newest good one.
type FailedDeployResource struct {
	DeployAttemptResource
	LastGoodImage string `json:"last_good_image,omitempty"`
}

// ListFailedDeploys calls GET /api/v1/deploys/failed. since is a Go
// duration such as "24h"; empty uses the server default.
func (c *Client) ListFailedDeploys(ctx context.Context, since string) ([]FailedDeployResource, error) {
	path := "/api/v1/deploys/failed"
	if since != "" {
		path += "?since=" + url.QueryEscape(since)
	}
	var out []FailedDeployResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

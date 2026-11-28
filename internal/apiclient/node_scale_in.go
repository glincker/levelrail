package apiclient

import (
	"context"
	"net/http"

	"github.com/glincker/levelrail/internal/nodescale"
)

// GetNodeScaleInPreview calls GET /api/v1/nodes/scale-in/preview: read only, nothing is destroyed.
func (c *Client) GetNodeScaleInPreview(ctx context.Context) (nodescale.Preview, error) {
	var out nodescale.Preview
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/scale-in/preview", nil, &out)
	return out, err
}

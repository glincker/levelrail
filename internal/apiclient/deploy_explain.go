package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/deploytimeline"
)

// ExplainDeploy returns the rule-based failure explanation for one deploy.
func (c *Client) ExplainDeploy(ctx context.Context, id string) (deploytimeline.Explanation, error) {
	var out deploytimeline.Explanation
	err := c.do(ctx, http.MethodGet, "/api/v1/deployments/"+url.PathEscape(id)+"/explain", nil, &out)
	return out, err
}

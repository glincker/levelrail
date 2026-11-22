package apiclient

import (
	"context"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/deploytimeline"
)

// ExplainDeploy returns the rule-based failure explanation for one deploy.
func (c *Client) ExplainDeploy(ctx context.Context, id string) (deploytimeline.Explanation, error) {
	return getJSON[deploytimeline.Explanation](ctx, c, "/api/v1/deployments/"+url.PathEscape(id)+"/explain")
}

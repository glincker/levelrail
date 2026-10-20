package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AppHealthScoreCategory mirrors internal/api's healthScoreCategory.
type AppHealthScoreCategory struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// AppHealthScoreResource mirrors internal/api's appHealthScoreResource.
type AppHealthScoreResource struct {
	AppName    string                   `json:"app_name"`
	Status     string                   `json:"status"`
	Categories []AppHealthScoreCategory `json:"categories"`
	ComputedAt time.Time                `json:"computed_at"`
}

// GetAppHealthScore calls GET /api/v1/apps/{name}/health-score: a live
// synthesis of deploy, security, resilience, and observability signals
// into one pass/warn/fail verdict per category.
func (c *Client) GetAppHealthScore(ctx context.Context, name string) (AppHealthScoreResource, error) {
	var out AppHealthScoreResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/health-score", nil, &out)
	return out, err
}

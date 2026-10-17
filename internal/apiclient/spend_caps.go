package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// AppBudget is GET/PUT /api/v1/apps/{name}/budget.
type AppBudget struct {
	spendcap.Status
	CostPriced bool `json:"cost_priced"`
}

// AppBudgetUpdate is the PUT body; it replaces the whole budget.
type AppBudgetUpdate struct {
	MonthlyEgressBytes int64   `json:"monthly_egress_bytes"`
	MonthlyCostCeiling float64 `json:"monthly_cost_ceiling"`
	Thresholds         []int   `json:"thresholds,omitempty"`
	Action             string  `json:"action"`
	ThrottleRPS        int     `json:"throttle_rps,omitempty"`
}

// AppBudgetHistory is GET /api/v1/apps/{name}/budget/history.
type AppBudgetHistory struct {
	Events []spendcap.Event `json:"events"`
}

func appBudgetPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/budget"
}

// GetAppBudget calls GET /api/v1/apps/{name}/budget; an app with no budget
// fails with an APIError of status 404.
func (c *Client) GetAppBudget(ctx context.Context, app string) (AppBudget, error) {
	var out AppBudget
	err := c.do(ctx, http.MethodGet, appBudgetPath(app), nil, &out)
	return out, err
}

// SetAppBudget calls PUT /api/v1/apps/{name}/budget.
func (c *Client) SetAppBudget(ctx context.Context, app string, u AppBudgetUpdate) (AppBudget, error) {
	var out AppBudget
	err := c.do(ctx, http.MethodPut, appBudgetPath(app), u, &out)
	return out, err
}

// ClearAppBudget calls DELETE /api/v1/apps/{name}/budget.
func (c *Client) ClearAppBudget(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, appBudgetPath(app), nil, nil)
}

// GetAppBudgetHistory calls GET /api/v1/apps/{name}/budget/history.
func (c *Client) GetAppBudgetHistory(ctx context.Context, app string) (AppBudgetHistory, error) {
	var out AppBudgetHistory
	err := c.do(ctx, http.MethodGet, appBudgetPath(app)+"/history", nil, &out)
	return out, err
}

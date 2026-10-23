package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// AppBudget is GET/PUT /api/v1/apps/{name}/budget.
type AppBudget struct {
	spendcap.Status
	CostPriced bool            `json:"cost_priced"`
	Forecast   *SpendForecast  `json:"forecast,omitempty"`
	Guard      *SpendGuardInfo `json:"guard,omitempty"`
}

// SpendForecast is the projected month end position of a cap.
type SpendForecast struct {
	Cost        usagecost.Band `json:"cost"`
	EgressBytes int64          `json:"egress_bytes"`
	Pct         float64        `json:"pct"`
	WillExceed  bool           `json:"will_exceed"`
}

// SpendGuardReason is one cap or budget holding an app at its limit.
type SpendGuardReason struct {
	Source string  `json:"source"`
	Pct    float64 `json:"pct"`
}

// SpendGuardInfo says whether new deploys of an app are refused.
type SpendGuardInfo struct {
	Blocked  bool               `json:"blocked"`
	Allowed  bool               `json:"allowed"`
	Reasons  []SpendGuardReason `json:"reasons"`
	Override *spendcap.Override `json:"override,omitempty"`
}

// AppBudgets is GET /api/v1/costs/app-caps.
type AppBudgets struct {
	Caps []AppBudget `json:"caps"`
}

// SpendDryRun is the answer to a pause dry-run; nothing was changed.
type SpendDryRun struct {
	spendcap.Preview
	Scope    string `json:"scope"`
	Target   string `json:"target"`
	Currency string `json:"currency"`
	Note     string `json:"note"`
}

// AppBudgetUpdate is the PUT body; it replaces the whole budget.
type AppBudgetUpdate struct {
	MonthlyEgressBytes int64   `json:"monthly_egress_bytes"`
	MonthlyCostCeiling float64 `json:"monthly_cost_ceiling"`
	Thresholds         []int   `json:"thresholds,omitempty"`
	Action             string  `json:"action"`
	ThrottleRPS        int     `json:"throttle_rps,omitempty"`
	GuardMode          string  `json:"guard_mode,omitempty"`
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

// ListAppBudgets calls GET /api/v1/costs/app-caps.
func (c *Client) ListAppBudgets(ctx context.Context) (AppBudgets, error) {
	var out AppBudgets
	err := c.do(ctx, http.MethodGet, "/api/v1/costs/app-caps", nil, &out)
	return out, err
}

// DryRunAppBudget calls POST /api/v1/apps/{name}/budget/dry-run.
func (c *Client) DryRunAppBudget(ctx context.Context, app string) (SpendDryRun, error) {
	var out SpendDryRun
	err := c.do(ctx, http.MethodPost, appBudgetPath(app)+"/dry-run", nil, &out)
	return out, err
}

// DryRunCostBudget calls POST /api/v1/costs/budgets/{id}/dry-run.
func (c *Client) DryRunCostBudget(ctx context.Context, id string) (SpendDryRun, error) {
	var out SpendDryRun
	err := c.do(ctx, http.MethodPost, "/api/v1/costs/budgets/"+PathEscape(id)+"/dry-run", nil, &out)
	return out, err
}

// OverrideSpendGuard calls POST /api/v1/apps/{name}/budget/override.
func (c *Client) OverrideSpendGuard(ctx context.Context, app, reason string) (SpendGuardInfo, error) {
	var out SpendGuardInfo
	err := c.do(ctx, http.MethodPost, appBudgetPath(app)+"/override", map[string]string{"reason": reason}, &out)
	return out, err
}

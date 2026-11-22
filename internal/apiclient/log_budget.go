package apiclient

import (
	"context"
	"net/http"
)

// LogUsageApp is one app's stored log volume against its budget.
type LogUsageApp struct {
	App          string   `json:"app"`
	Bytes        int64    `json:"bytes"`
	Lines        int64    `json:"lines"`
	RateBytesMin float64  `json:"rate_bytes_per_min"`
	BudgetBytes  int64    `json:"budget_bytes"`
	BudgetCustom bool     `json:"budget_custom"`
	Percent      *float64 `json:"percent"`
	ForecastDays *float64 `json:"forecast_days"`
	TrimmedLines int64    `json:"trimmed_lines"`
	TrimmedBytes int64    `json:"trimmed_bytes"`
}

// LogUsage is the response of GET /api/v1/logs/usage.
type LogUsage struct {
	DefaultBudgetBytes int64         `json:"default_budget_bytes"`
	Apps               []LogUsageApp `json:"apps"`
}

// LogBudget is an app's log budget; MaxMB 0 means no override.
type LogBudget struct {
	MaxMB            int64 `json:"max_mb"`
	DefaultMB        int64 `json:"default_mb"`
	EffectiveMaxByte int64 `json:"effective_max_bytes"`
}

// GetLogUsage calls GET /api/v1/logs/usage.
func (c *Client) GetLogUsage(ctx context.Context) (LogUsage, error) {
	return getJSON[LogUsage](ctx, c, "/api/v1/logs/usage")
}

func logBudgetPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/log-budget"
}

// GetLogBudget calls GET /api/v1/apps/{name}/log-budget.
func (c *Client) GetLogBudget(ctx context.Context, app string) (LogBudget, error) {
	return getJSON[LogBudget](ctx, c, logBudgetPath(app))
}

// SetLogBudget calls PUT /api/v1/apps/{name}/log-budget.
func (c *Client) SetLogBudget(ctx context.Context, app string, maxMB int64) (LogBudget, error) {
	var out LogBudget
	err := c.do(ctx, http.MethodPut, logBudgetPath(app), struct {
		MaxMB int64 `json:"max_mb"`
	}{maxMB}, &out)
	return out, err
}

// DeleteLogBudget calls DELETE /api/v1/apps/{name}/log-budget.
func (c *Client) DeleteLogBudget(ctx context.Context, app string) error {
	return c.del(ctx, logBudgetPath(app))
}

// LogTrimResult is the response of POST /api/v1/apps/{name}/log-budget/trim.
type LogTrimResult struct {
	TrimmedLines int64 `json:"trimmed_lines"`
	TrimmedBytes int64 `json:"trimmed_bytes"`
	StillOver    bool  `json:"still_over"`
}

// TrimLogBudget calls POST /api/v1/apps/{name}/log-budget/trim.
func (c *Client) TrimLogBudget(ctx context.Context, app string) (LogTrimResult, error) {
	var out LogTrimResult
	err := c.do(ctx, http.MethodPost, logBudgetPath(app)+"/trim", struct{}{}, &out)
	return out, err
}

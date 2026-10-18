package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// CostExploration is GET /api/v1/costs/explore.
type CostExploration struct {
	usagecost.Exploration
	Granularity        string            `json:"granularity"`
	Currency           string            `json:"currency"`
	Period             usagecost.Period  `json:"period"`
	Labels             map[string]string `json:"labels"`
	Forecast           *usagecost.Band   `json:"forecast,omitempty"`
	Configured         bool              `json:"configured"`
	TelemetryAvailable bool              `json:"telemetry_available"`
	GeneratedAt        string            `json:"generated_at"`
}

// CostExploreQuery selects what the explorer aggregates.
type CostExploreQuery struct {
	GroupBy     string
	Granularity string
	Period      string
	From        string
	To          string
	Months      int
}

func (q CostExploreQuery) values() url.Values {
	v := url.Values{}
	for k, s := range map[string]string{"group_by": q.GroupBy, "granularity": q.Granularity, "period": q.Period, "from": q.From, "to": q.To} {
		if s != "" {
			v.Set(k, s)
		}
	}
	if q.Months > 0 {
		v.Set("months", strconv.Itoa(q.Months))
	}
	return v
}

// ExploreCosts calls GET /api/v1/costs/explore.
func (c *Client) ExploreCosts(ctx context.Context, q CostExploreQuery) (CostExploration, error) {
	path := "/api/v1/costs/explore"
	if v := q.values(); len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out CostExploration
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// CostBudgetStatus is one named budget with its month to date position.
type CostBudgetStatus struct {
	costbudget.Budget
	Currency         string         `json:"currency"`
	Period           string         `json:"period"`
	Spent            float64        `json:"spent"`
	Pct              float64        `json:"pct"`
	Forecast         usagecost.Band `json:"forecast"`
	ForecastPct      float64        `json:"forecast_pct"`
	Highest          int            `json:"highest"`
	Enforced         string         `json:"enforced"`
	Apps             []string       `json:"apps"`
	Level            string         `json:"level"`
	SuggestedMonthly float64        `json:"suggested_monthly"`
	ResetsAt         time.Time      `json:"resets_at"`
}

// CostBudgets is GET /api/v1/costs/budgets.
type CostBudgets struct {
	Budgets  []CostBudgetStatus `json:"budgets"`
	Currency string             `json:"currency"`
}

// ListCostBudgets calls GET /api/v1/costs/budgets.
func (c *Client) ListCostBudgets(ctx context.Context) (CostBudgets, error) {
	var out CostBudgets
	err := c.do(ctx, http.MethodGet, "/api/v1/costs/budgets", nil, &out)
	return out, err
}

// CreateCostBudget calls POST /api/v1/costs/budgets.
func (c *Client) CreateCostBudget(ctx context.Context, body map[string]any) (costbudget.Budget, error) {
	var out costbudget.Budget
	err := c.do(ctx, http.MethodPost, "/api/v1/costs/budgets", body, &out)
	return out, err
}

// UpdateCostBudget calls PUT /api/v1/costs/budgets/{id}; only the keys in body change.
func (c *Client) UpdateCostBudget(ctx context.Context, id string, body map[string]any) (costbudget.Budget, error) {
	var out costbudget.Budget
	err := c.do(ctx, http.MethodPut, "/api/v1/costs/budgets/"+PathEscape(id), body, &out)
	return out, err
}

// DeleteCostBudget calls DELETE /api/v1/costs/budgets/{id}.
func (c *Client) DeleteCostBudget(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/costs/budgets/"+PathEscape(id), nil, nil)
}

package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	costExplorerTool = "cost_explorer"
	costBudgetsTool  = "cost_budgets"
	costTopSeries    = 10
)

type costExplorerInput struct {
	GroupBy     string `json:"group_by,omitempty"`
	Granularity string `json:"granularity,omitempty"`
	Period      string `json:"period,omitempty"`
	Months      int    `json:"months,omitempty"`
}

type costBudgetsInput struct{}

func registerCostExplorerTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        costExplorerTool,
		Description: "Cost trend grouped by app, project, tag, node or kind, with month end forecast range and top movers.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in costExplorerInput) (*mcp.CallToolResult, map[string]any, error) {
		res, err := client.ExploreCosts(ctx, apiclient.CostExploreQuery{GroupBy: in.GroupBy, Granularity: in.Granularity, Period: in.Period, Months: in.Months})
		if err != nil {
			return nil, nil, fmt.Errorf("explore costs: %w", err)
		}
		series := res.Series
		if len(series) > costTopSeries {
			series = series[:costTopSeries]
		}
		return nil, map[string]any{
			"currency": res.Currency, "group_by": res.GroupBy, "granularity": res.Granularity,
			"buckets": res.Buckets, "trend": res.Trend, "total": res.Total, "previous": res.Previous,
			"series": series, "movers": res.Movers, "forecast": res.Forecast,
		}, nil
	})
	addTool(server, &mcp.Tool{
		Name:        costBudgetsTool,
		Description: "Cost budgets and per-app spend caps: spend, forecast, fired thresholds, deploy guard and enforcement state.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ costBudgetsInput) (*mcp.CallToolResult, map[string]any, error) {
		res, err := client.ListCostBudgets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list cost budgets: %w", err)
		}
		apps, err := client.ListAppBudgets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list app spend caps: %w", err)
		}
		return nil, map[string]any{"currency": res.Currency, "budgets": res.Budgets, "app_caps": apps.Caps}, nil
	})
}

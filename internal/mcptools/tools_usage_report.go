package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/usagecost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	usageReportTool    = "usage_report"
	usageReportTopRows = 10
)

type usageReportInput struct {
	Period string `json:"period,omitempty"`
}

type usageRow struct {
	Name     string   `json:"name"`
	Total    float64  `json:"total"`
	Previous float64  `json:"previous"`
	DeltaPct *float64 `json:"delta_pct,omitempty"`
}

func usageRows(groups []usagecost.Group, labels map[string]string, dim string) []usageRow {
	out := make([]usageRow, 0, min(len(groups), usageReportTopRows))
	for _, g := range groups {
		if len(out) == usageReportTopRows {
			break
		}
		name := g.Name
		if dim != usagecost.DimResource {
			if name = labels[dim+":"+g.Key]; name == "" {
				name = g.Key
			}
		}
		out = append(out, usageRow{Name: name, Total: g.Total, Previous: g.Previous, DeltaPct: g.DeltaPct})
	}
	return out
}

func registerUsageReportTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        usageReportTool,
		Description: "Cost showback per project, team and app, with forecast.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in usageReportInput) (*mcp.CallToolResult, map[string]any, error) {
		rep, err := client.GetUsageReport(ctx, apiclient.UsageReportQuery{Period: in.Period})
		if err != nil {
			return nil, nil, fmt.Errorf("get usage report: %w", err)
		}
		return nil, map[string]any{
			"currency": rep.Currency, "from": usagecost.DayKey(rep.Period.From), "to": usagecost.DayKey(rep.Period.To.AddDate(0, 0, -1)),
			"total": rep.Totals.Total, "unallocated": rep.Totals.Unallocated, "forecast": rep.Forecast,
			"projects":  usageRows(rep.Projects, rep.Labels, usagecost.DimProject),
			"teams":     usageRows(rep.Teams, rep.Labels, usagecost.DimTeam),
			"resources": usageRows(rep.Resources, rep.Labels, usagecost.DimResource),
		}, nil
	})
}

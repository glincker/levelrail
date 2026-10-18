package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listDashboardsInput struct {
	Project string `json:"project,omitempty" jsonschema:"only dashboards of this project id"`
}

type getDashboardInput struct {
	ID string `json:"id" jsonschema:"the dashboard id from list_dashboards"`
}

type dashboardSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id,omitempty"`
	IsDefault bool   `json:"is_default"`
}

type dashboardDetail struct {
	dashboardSummary
	Document string `json:"document" jsonschema:"the portable dashboard JSON: time range, refresh and widgets with their targets"`
}

type dashboardList struct {
	Dashboards []dashboardSummary `json:"dashboards"`
}

func registerDashboardTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_dashboards",
		Description: "List the custom dashboards visible to the caller (their own plus shared ones), default dashboards first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listDashboardsInput) (*mcp.CallToolResult, dashboardList, error) {
		rows, err := client.ListDashboards(ctx, in.Project)
		if err != nil {
			return nil, dashboardList{}, fmt.Errorf("list dashboards: %w", err)
		}
		out := dashboardList{Dashboards: make([]dashboardSummary, 0, len(rows))}
		for _, d := range rows {
			out.Dashboards = append(out.Dashboards, summarizeDashboard(d))
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_dashboard",
		Description: "Get one custom dashboard with its widget document (which apps, databases and nodes each widget reads from).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDashboardInput) (*mcp.CallToolResult, dashboardDetail, error) {
		d, err := client.GetDashboard(ctx, in.ID)
		if err != nil {
			return nil, dashboardDetail{}, fmt.Errorf("get dashboard: %w", err)
		}
		return nil, dashboardDetail{dashboardSummary: summarizeDashboard(d), Document: string(d.Doc)}, nil
	})
}

func summarizeDashboard(d apiclient.DashboardResource) dashboardSummary {
	return dashboardSummary{ID: d.ID, Name: d.Name, Scope: d.Scope, ProjectID: d.ProjectID, IsDefault: d.IsDefault}
}

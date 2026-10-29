package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAnalysisTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_analysis_report",
		Description: "Get an analysis report by id, or list an app's. Read only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in analysisReportInput) (*mcp.CallToolResult, any, error) {
		if in.ID == "" {
			rows, err := client.ListAnalysisReports(ctx, in.App)
			if err != nil {
				return nil, nil, fmt.Errorf("list analysis reports for app %q: %w", in.App, err)
			}
			return nil, map[string]any{"reports": rows}, nil
		}
		rep, err := client.GetAnalysisReport(ctx, in.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("get analysis report %q: %w", in.ID, err)
		}
		return nil, rep, nil
	})
}

type analysisReportInput struct {
	ID  string `json:"id,omitempty"`
	App string `json:"app,omitempty"`
}

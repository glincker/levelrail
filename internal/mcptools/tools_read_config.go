package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type monitorResultsInput struct {
	ID     string `json:"id" jsonschema:"the monitor ID from list_monitors"`
	Window string `json:"window,omitempty" jsonschema:"how far back to read, e.g. 1h, 24h or 7d (server default when empty)"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum raw probe results, newest first"`
}

// Read-only views of what the dashboard shows. Each calls the same REST route the
// dashboard uses, so abilities, org scoping and redaction stay server-side.
func registerReadConfigTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_deploy_policies",
		Description: "List deploy policies (scope, scope ID and the rules that gate deploys). Dry-run what they would block with preflight_app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DeployPolicy, error) {
		out, err := client.ListDeployPolicies(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploy policies: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_monitors",
		Description: "List uptime monitors (kind, target, interval, expected status, status page link). Heartbeat secrets are never returned.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.Monitor, error) {
		out, err := client.ListMonitors(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list monitors: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_monitor_results",
		Description: "Get one monitor's recent probe results and hourly rollups (failures, latency) for a window.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in monitorResultsInput) (*mcp.CallToolResult, apiclient.MonitorResults, error) {
		out, err := client.MonitorResultsFor(ctx, in.ID, in.Window, in.Limit)
		if err != nil {
			return nil, apiclient.MonitorResults{}, fmt.Errorf("get monitor results %q: %w", in.ID, err)
		}
		return nil, out, nil
	})
}

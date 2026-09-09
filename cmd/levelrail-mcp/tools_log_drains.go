package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerLogDrainTools registers only the read side of an app's log
// drain. Configuring or clearing a drain changes where an operator's
// container logs get forwarded externally, so that stays a CLI/dashboard
// action, matching registerWebhookTools' own read-only rationale.
func registerLogDrainTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_log_drain",
		Description: "Get an app's configured external log drain, if any: sink type (http or syslog), target, and whether forwarding is enabled. This is additive to the built-in node-local log store, not a replacement, so a missing or disabled drain does not mean logs aren't being collected at all. Read-only; does not configure or clear a drain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LogDrainResource, error) {
		drain, err := client.GetLogDrain(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LogDrainResource{}, fmt.Errorf("get log drain for app %q: %w", in.Name, err)
		}
		return nil, drain, nil
	})
}

package mcptools

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
	addTool(server, &mcp.Tool{
		Name:        "get_app_log_drain",
		Description: "An app's external log drain, if any: sink type, target, enabled. Additive to the node-local log store.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LogDrainResource, error) {
		drain, err := client.GetLogDrain(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LogDrainResource{}, fmt.Errorf("get log drain for app %q: %w", in.Name, err)
		}
		return nil, drain, nil
	})
}

package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// registerDeployFreezeTools adds the read-only view of an app's deploy
// freeze windows. Changing a freeze stays with the dashboard and CLI.
func registerDeployFreezeTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_deploy_freeze",
		Description: "Show an app's deploy freeze windows (its own and inherited global ones) and whether a freeze is active right now. While frozen, webhook and pipeline deploys are held and released when the window ends; manual deploys need an explicit override.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.DeployFreezeResource, error) {
		res, err := client.GetDeployFreeze(ctx, in.Name)
		if err != nil {
			return nil, apiclient.DeployFreezeResource{}, fmt.Errorf("get deploy freeze for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})
}

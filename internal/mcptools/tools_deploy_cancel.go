package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type cancelDeployInput struct {
	Name     string `json:"name" jsonschema:"the app's name"`
	DeployID string `json:"deploy_id" jsonschema:"deploy attempt id, from list_deploy_attempts"`
}

func registerDeployCancelTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "cancel_deploy",
		Description: "Cancel a queued or in-progress deploy; the serving release is untouched. Fails after cutover (use rollback_app).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cancelDeployInput) (*mcp.CallToolResult, apiclient.DeployAttemptResource, error) {
		attempt, err := client.CancelDeploy(ctx, in.Name, in.DeployID)
		if err != nil {
			return nil, apiclient.DeployAttemptResource{}, fmt.Errorf("cancel deploy %q of app %q: %w", in.DeployID, in.Name, err)
		}
		return nil, attempt, nil
	})
}

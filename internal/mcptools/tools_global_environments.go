package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const globalEnvironmentsGroup = "global-environments"

func registerGlobalEnvironmentTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_global_environments",
		Description: "List every environment (dev, test, uat, production, custom, preview) with kind, protected flag and app and database counts. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.GlobalEnvironmentResource, error) {
		envs, err := client.ListAllEnvironments(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list environments: %w", err)
		}
		return nil, envs, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "move_app_environment",
		Description: "Move an app to another environment. A move into or out of a protected environment needs confirm true and then returns a pending_approval that a human must approve; it never applies by itself. Blocked during a deploy freeze.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in moveAppEnvironmentInput) (*mcp.CallToolResult, apiclient.MoveEnvironmentResult, error) {
		res, err := client.MoveAppEnvironment(ctx, in.Name, apiclient.MoveEnvironmentRequest{EnvironmentID: in.EnvironmentID, Confirm: in.Confirm})
		if err != nil {
			return nil, apiclient.MoveEnvironmentResult{}, fmt.Errorf("move app %q to environment %q: %w", in.Name, in.EnvironmentID, err)
		}
		return nil, res, nil
	})
}

type moveAppEnvironmentInput struct {
	Name          string `json:"name" jsonschema:"the app's name"`
	EnvironmentID string `json:"environment_id" jsonschema:"target environment id from list_global_environments, empty to untag"`
	Confirm       bool   `json:"confirm,omitempty" jsonschema:"true to request a move that touches a protected environment"`
}

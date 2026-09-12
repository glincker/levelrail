package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerEnvironmentTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_environments",
		Description: "List a project's staging/production-style environment labels: name and whether it's protected (deploying into a protected environment requires the caller to pass confirm=true to deploy_app/rollback_app). Read-only; does not create, edit, or delete an environment.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projectIDInput) (*mcp.CallToolResult, []apiclient.EnvironmentResource, error) {
		environments, err := client.ListEnvironments(ctx, in.ProjectID)
		if err != nil {
			return nil, nil, fmt.Errorf("list environments for project %q: %w", in.ProjectID, err)
		}
		return nil, environments, nil
	})
}

type projectIDInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project's id, from list_projects"`
}

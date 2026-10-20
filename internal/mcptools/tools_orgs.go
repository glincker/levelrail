package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerOrganizationTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_organizations",
		Description: "Organizations, a plain label grouping projects. No owners or members; every operator sees all.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.OrganizationResource, error) {
		orgs, err := client.ListOrganizations(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list organizations: %w", err)
		}
		return nil, orgs, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List every project on the control plane: a lightweight, non-auth label an app or database can be filed under, optionally itself filed under an organization. There is no owner, member list, or per-project ability.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ProjectResource, error) {
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list projects: %w", err)
		}
		return nil, projects, nil
	})
}

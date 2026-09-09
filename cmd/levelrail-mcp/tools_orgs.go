package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerOrganizationTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_organizations",
		Description: "List every organization on the control plane: a lightweight, non-auth label that groups projects. There is no owner, member list, or per-organization ability; every operator already sees every organization's projects and apps regardless of grouping. Read-only; does not create or delete an organization.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.OrganizationResource, error) {
		orgs, err := client.ListOrganizations(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list organizations: %w", err)
		}
		return nil, orgs, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_projects",
		Description: "List every project on the control plane: a lightweight, non-auth label an app or database can be filed under, optionally itself filed under an organization. There is no owner, member list, or per-project ability. Read-only; does not create or delete a project.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ProjectResource, error) {
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list projects: %w", err)
		}
		return nil, projects, nil
	})
}

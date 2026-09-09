package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDomainTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_domains",
		Description: "List every domain routed by this control plane, across every app: domain name and which app owns it. The same data DomainEditor shows per-app, aggregated into one cross-app read. Read-only; does not connect, edit, or remove a domain.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DomainResource, error) {
		domains, err := client.ListDomains(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list domains: %w", err)
		}
		return nil, domains, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_network",
		Description: "Get an app's live traffic path: the container's declared port, the current Docker-assigned host port Caddy is actually proxying to, and whether the container is running. Useful for diagnosing why a domain isn't reaching an app. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.NetworkResource, error) {
		network, err := client.GetAppNetwork(ctx, in.Name)
		if err != nil {
			return nil, apiclient.NetworkResource{}, fmt.Errorf("get network for app %q: %w", in.Name, err)
		}
		return nil, network, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_domain_maintenance_status",
		Description: "Get whether one of an app's domains currently has maintenance mode enabled, showing a static page instead of proxying to the app's container. Read-only; does not enable or disable maintenance mode.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, apiclient.DomainMaintenanceResource, error) {
		status, err := client.GetDomainMaintenance(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, apiclient.DomainMaintenanceResource{}, fmt.Errorf("get maintenance status for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, status, nil
	})
}

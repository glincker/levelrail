package mcptools

import (
	"context"
	"fmt"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDomainTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_domains",
		Description: "List every routed domain across all apps, with the owning app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DomainResource, error) {
		domains, err := client.ListDomains(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list domains: %w", err)
		}
		return nil, domains, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_network",
		Description: "An app's live traffic path: declared port, proxy target host port, and whether the container runs.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.NetworkResource, error) {
		network, err := client.GetAppNetwork(ctx, in.Name)
		if err != nil {
			return nil, apiclient.NetworkResource{}, fmt.Errorf("get network for app %q: %w", in.Name, err)
		}
		return nil, network, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_domain_maintenance_status",
		Description: "Whether an app domain has maintenance mode on (a static page instead of the app), since when and until when.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, apiclient.DomainMaintenanceResource, error) {
		status, err := client.GetDomainMaintenance(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, apiclient.DomainMaintenanceResource{}, fmt.Errorf("get maintenance status for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, status, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_domain_redirect",
		Description: "Whether an app domain redirects to a target URL instead of proxying, with target and status (301 or 302).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, apiclient.DomainRedirectResource, error) {
		status, err := client.GetDomainRedirect(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, apiclient.DomainRedirectResource{}, fmt.Errorf("get redirect status for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, status, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_domain_error_pages",
		Description: "Custom error pages of one app domain: status codes with a custom HTML body, and that body.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, apiclient.DomainErrorPagesResource, error) {
		pages, err := client.GetDomainErrorPages(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, apiclient.DomainErrorPagesResource{}, fmt.Errorf("get error pages for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, pages, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "check_domain_dns",
		Description: "Real DNS lookup for an app's domain: connected, not_resolving, resolves_elsewhere or unconfigured.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, apiclient.DomainCheckResource, error) {
		result, err := client.CheckDomain(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, apiclient.DomainCheckResource{}, fmt.Errorf("check dns for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, result, nil
	})
}

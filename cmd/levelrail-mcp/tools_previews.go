package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPreviewTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_preview_environments",
		Description: "List an app's active pull-request preview environments: PR number, branch, head commit, domain, status, and whether it's stale (old enough that the TTL sweep would tear it down on its next tick). Read-only; does not create or tear down a preview.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.PreviewEnvironmentResource, error) {
		previews, err := client.ListPreviewEnvironments(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list preview environments for app %q: %w", in.Name, err)
		}
		return nil, previews, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sweep_stale_preview_environments",
		Description: "Tear down every preview environment, across every app, whose pull-request-closed webhook never arrived (a failed delivery, see list_webhook_deliveries) and has gone stale past the configured TTL. Manual trigger for the same fallback that already runs automatically in the background; returns how many were torn down.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.SweepPreviewEnvironmentsResult, error) {
		result, err := client.SweepPreviewEnvironments(ctx)
		if err != nil {
			return nil, apiclient.SweepPreviewEnvironmentsResult{}, fmt.Errorf("sweep stale preview environments: %w", err)
		}
		return nil, result, nil
	})
}

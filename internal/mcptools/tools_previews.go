package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPreviewTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_preview_environments",
		Description: "An app's pull-request previews: PR, branch, commit, domain, status, stale flag, preview databases.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.PreviewEnvironmentResource, error) {
		previews, err := client.ListPreviewEnvironments(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list preview environments for app %q: %w", in.Name, err)
		}
		return nil, previews, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "sweep_stale_preview_environments",
		Description: "Tear down stale preview environments, across all apps, whose pull-request-closed webhook never arrived. Manual trigger for the automatic fallback; returns the count.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.SweepPreviewEnvironmentsResult, error) {
		result, err := client.SweepPreviewEnvironments(ctx)
		if err != nil {
			return nil, apiclient.SweepPreviewEnvironmentsResult{}, fmt.Errorf("sweep stale preview environments: %w", err)
		}
		return nil, result, nil
	})
}

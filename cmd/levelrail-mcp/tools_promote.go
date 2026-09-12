package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPromoteTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "preview_promote_app",
		Description: "Show what promote_app would change, without applying it: the source and target apps, their current images, and the resulting image diff. Only the image tag is ever compared; env vars, ports, domains, and resource limits are the target app's own settings and are never part of this diff. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in promoteAppInput) (*mcp.CallToolResult, apiclient.PromotePreviewResource, error) {
		preview, err := client.PreviewPromotion(ctx, in.Name, in.To, in.Target)
		if err != nil {
			return nil, apiclient.PromotePreviewResource{}, fmt.Errorf("preview promotion for app %q: %w", in.Name, err)
		}
		return nil, preview, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "promote_app",
		Description: "Point a sibling app's image at name's current image and redeploy it, through the same mechanism deploy_app uses. The sibling app is found in the same project as name: target names it explicitly, or it's auto-discovered when exactly one app tagged with the destination environment belongs to that project. Asynchronous: use get_app_status on the target app to watch it converge. Promoting into a protected environment requires confirm true, the same gate deploy_app enforces.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in promoteAppInput) (*mcp.CallToolResult, apiclient.AppResource, error) {
		app, err := client.PromoteApp(ctx, in.Name, apiclient.PromoteAppRequest{To: in.To, Target: in.Target, Confirm: in.Confirm})
		if err != nil {
			return nil, apiclient.AppResource{}, fmt.Errorf("promote app %q to environment %q: %w", in.Name, in.To, err)
		}
		return nil, app, nil
	})
}

type promoteAppInput struct {
	Name    string `json:"name" jsonschema:"the source app's name"`
	To      string `json:"to" jsonschema:"destination environment id"`
	Target  string `json:"target,omitempty" jsonschema:"target app name, to disambiguate when more than one app in the destination environment belongs to the same project; omit to auto-discover the sole candidate"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"required true to promote into an app tagged with a protected environment; omit or false fails with a 409 naming the environment if it's protected"`
}

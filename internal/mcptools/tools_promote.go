package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPromoteTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "preview_promote_app",
		Description: "Preview promote_app without applying it: source and target apps, their images, and the diff of image, replicas, resources, health and env key names (never values). Lists blockers and whether confirmation is needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in promoteAppInput) (*mcp.CallToolResult, apiclient.PromotePreviewResource, error) {
		preview, err := client.PreviewPromotion(ctx, in.Name, in.To, in.Target)
		if err != nil {
			return nil, apiclient.PromotePreviewResource{}, fmt.Errorf("preview promotion for app %q: %w", in.Name, err)
		}
		return nil, preview, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "promote_app",
		Description: "Point a sibling app (target, or the one app in the destination environment) at this app's image and redeploy, like deploy_app. Asynchronous: watch with get_app_status. A production or protected destination needs confirm true and returns pending_approval until a different privileged human runs approve_deploy_approval.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in promoteAppInput) (*mcp.CallToolResult, apiclient.DeployTriggerResult, error) {
		result, err := client.PromoteApp(ctx, in.Name, apiclient.PromoteAppRequest{To: in.To, Target: in.Target, Confirm: in.Confirm, IncludeEnv: in.IncludeEnv, Force: in.Force})
		if err != nil {
			return nil, apiclient.DeployTriggerResult{}, fmt.Errorf("promote app %q to environment %q: %w", in.Name, in.To, err)
		}
		return nil, result, nil
	})
}

type promoteAppInput struct {
	Name       string `json:"name" jsonschema:"the source app's name"`
	To         string `json:"to" jsonschema:"destination environment id"`
	Target     string `json:"target,omitempty" jsonschema:"target app name, to disambiguate when more than one app in the destination environment belongs to the same project; omit to auto-discover the sole candidate"`
	Confirm    bool   `json:"confirm,omitempty" jsonschema:"required true to promote into an app tagged with a protected environment; omit or false fails with a 409 naming the environment if it's protected"`
	IncludeEnv bool   `json:"include_env,omitempty" jsonschema:"also apply added and removed plain env keys; values never overwrite the target"`
	Force      bool   `json:"force,omitempty" jsonschema:"promote even if the source is unhealthy or its last deploy failed"`
}

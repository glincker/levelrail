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
		Description: "Show what promote_app would change, without applying it: the source and target apps, their current images, and the resulting image diff. The diff covers image, replicas, resources, health and env key names (never values); domains and secret values are never touched. It also lists blockers (unhealthy source, failed last deploy) and whether confirmation is needed. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in promoteAppInput) (*mcp.CallToolResult, apiclient.PromotePreviewResource, error) {
		preview, err := client.PreviewPromotion(ctx, in.Name, in.To, in.Target)
		if err != nil {
			return nil, apiclient.PromotePreviewResource{}, fmt.Errorf("preview promotion for app %q: %w", in.Name, err)
		}
		return nil, preview, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "promote_app",
		Description: "Point a sibling app's image at name's current image and redeploy it, through the same mechanism deploy_app uses. The sibling app is found in the same project as name: target names it explicitly, or it's auto-discovered when exactly one app tagged with the destination environment belongs to that project. Asynchronous: use get_app_status on the target app to watch it converge. Promoting into a production or protected environment requires confirm true just to be accepted at all, and even then the result's pending_approval is set instead of the promotion actually applying: a different, sufficiently privileged human must approve it (approve_deploy_approval) first.",
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

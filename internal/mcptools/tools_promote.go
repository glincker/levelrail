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
		Description: "Preview promote_app without applying: image and config diff (env key names only), blockers and whether confirm is needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in promoteAppInput) (*mcp.CallToolResult, apiclient.PromotePreviewResource, error) {
		preview, err := client.PreviewPromotion(ctx, in.Name, in.To, in.Target)
		if err != nil {
			return nil, apiclient.PromotePreviewResource{}, fmt.Errorf("preview promotion for app %q: %w", in.Name, err)
		}
		return nil, preview, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "promote_app",
		Description: "Point a sibling app (target, or the sole app in the destination environment) at this app's image and redeploy. Asynchronous: watch get_app_status. A protected destination needs confirm and waits for approve_deploy_approval.",
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
	Target     string `json:"target,omitempty" jsonschema:"target app name when the destination environment has several; omit to auto-pick the only one"`
	Confirm    bool   `json:"confirm,omitempty" jsonschema:"true to confirm a protected environment, else 409"`
	IncludeEnv bool   `json:"include_env,omitempty" jsonschema:"also apply added and removed plain env keys; values never overwrite the target"`
	Force      bool   `json:"force,omitempty" jsonschema:"promote even if the source is unhealthy or its last deploy failed"`
}

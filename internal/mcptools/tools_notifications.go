package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerNotificationTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_notification_channels",
		Description: "Global notification channels: name, kind, enabled, destination URL. Alert rules and deploy targets attach by channel_id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.NotificationChannelResource, error) {
		channels, err := client.ListNotificationChannels(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list notification channels: %w", err)
		}
		return nil, channels, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_notification_deliveries",
		Description: "List a notification channel's recorded send history, newest first: deploy outcomes, alert rule firings, and test sends alike, with success/failure and error for each.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in notificationDeliveriesInput) (*mcp.CallToolResult, []apiclient.NotificationDeliveryResource, error) {
		deliveries, err := client.ListNotificationDeliveries(ctx, in.ID, in.Limit)
		if err != nil {
			return nil, nil, fmt.Errorf("list notification deliveries for channel %q: %w", in.ID, err)
		}
		return nil, deliveries, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_deploy_notify_targets",
		Description: "An app's deploy-outcome notification targets: channel_id, resolved notify_url and kind. Includes disabled ones.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.DeployNotifyTargetResource, error) {
		targets, err := client.ListDeployNotifyTargets(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploy notify targets for app %q: %w", in.Name, err)
		}
		return nil, targets, nil
	})
}

type notificationDeliveriesInput struct {
	ID    string `json:"id" jsonschema:"the notification channel's id"`
	Limit int    `json:"limit,omitempty" jsonschema:"max rows to return, default the server's own default"`
}

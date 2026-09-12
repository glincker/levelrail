package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerNotificationTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notification_channels",
		Description: "List every global notification channel connected to the control plane: name, kind (slack, discord, telegram, generic webhook, email...), enabled state, and the destination URL. Deploy-notify-targets and alert rules attach to one of these by channel_id. Read-only; does not create, delete, or test a channel.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.NotificationChannelResource, error) {
		channels, err := client.ListNotificationChannels(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list notification channels: %w", err)
		}
		return nil, channels, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notification_deliveries",
		Description: "List a notification channel's recorded send history, newest first: deploy outcomes, alert rule firings, and test sends alike, with success/failure and error for each. Read-only; does not send a new notification.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in notificationDeliveriesInput) (*mcp.CallToolResult, []apiclient.NotificationDeliveryResource, error) {
		deliveries, err := client.ListNotificationDeliveries(ctx, in.ID, in.Limit)
		if err != nil {
			return nil, nil, fmt.Errorf("list notification deliveries for channel %q: %w", in.ID, err)
		}
		return nil, deliveries, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_deploy_notify_targets",
		Description: "List an app's deploy-outcome notification targets: each one attaches a notification channel (channel_id, and its resolved notify_url/notify_kind) so a deploy success or failure sends a message there. Includes disabled targets. Read-only; does not create, edit, or delete a target.",
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

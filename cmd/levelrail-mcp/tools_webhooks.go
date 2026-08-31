package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerWebhookTools registers only the read side of webhook
// deliveries. Replaying a stored delivery can trigger a real build and
// deploy, and unlike deploy_app/rollback_app there is no close
// precedent for a write tool here, so replay is left to the CLI/dashboard.
func registerWebhookTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_webhook_deliveries",
		Description: "List an app's recent inbound git-provider webhook requests, newest first, whether or not they verified or matched a connected git source. Useful for debugging why a push didn't trigger a deploy. Read-only; does not replay a delivery.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in webhookDeliveriesInput) (*mcp.CallToolResult, []apiclient.WebhookDeliveryResource, error) {
		deliveries, err := client.ListWebhookDeliveries(ctx, in.Name, apiclient.ListWebhookDeliveriesOptions{
			Limit:  in.Limit,
			Before: in.Before,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list webhook deliveries for app %q: %w", in.Name, err)
		}
		return nil, deliveries, nil
	})
}

type webhookDeliveriesInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max deliveries to return, default the server's own default"`
	Before string `json:"before,omitempty" jsonschema:"only return deliveries received before this RFC3339 timestamp, for paging backward"`
}

package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAlertTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_alert_rules",
		Description: "List an app's configured alert rules (threshold, crashloop, cert_expiry, and scheduled_task_failure kinds), including disabled ones and each rule's current firing state. A cert_expiry rule watches every certificate on the control plane platform-wide, not just this app's own domains. Surfaces what's already configured; does not create, edit, or delete a rule.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.AlertRuleResource, error) {
		rules, err := client.ListAlertRules(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list alert rules for app %q: %w", in.Name, err)
		}
		return nil, rules, nil
	})
}

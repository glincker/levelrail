package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAlertTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_alert_rules",
		Description: "List an app's alert rules of every kind, disabled included, with firing state. cert_expiry rules watch all certificates.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.AlertRuleResource, error) {
		rules, err := client.ListAlertRules(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list alert rules for app %q: %w", in.Name, err)
		}
		return nil, rules, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_slo_status",
		Description: "An app's error budget remaining and burn rate per alert tier for a request based SLO (availability, or latency with latency_ms).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in sloStatusInput) (*mcp.CallToolResult, *apiclient.SLOPreviewResource, error) {
		cfg := apiclient.AlertSLOConfig{Objective: "availability", Target: in.Target, LatencyMs: in.LatencyMs}
		if in.LatencyMs > 0 {
			cfg.Objective = "latency"
		}
		out, err := client.GetSLOPreview(ctx, in.Name, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("get slo status for app %q: %w", in.Name, err)
		}
		return nil, out, nil
	})
}

type sloStatusInput struct {
	Name      string  `json:"name" jsonschema:"the app's name"`
	Target    float64 `json:"target,omitempty" jsonschema:"SLO target as a percentage of good requests (default 99.9)"`
	LatencyMs float64 `json:"latency_ms,omitempty" jsonschema:"make it a latency SLO: a request is good when it finishes within this many milliseconds"`
}

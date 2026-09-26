package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listSilencesInput struct {
	IncludeExpired bool `json:"include_expired,omitempty" jsonschema:"also list expired silences (kept as history)"`
}

type createSilenceInput struct {
	RuleIDs    []string          `json:"rule_ids,omitempty" jsonschema:"only alerts from these rule IDs"`
	Apps       []string          `json:"apps,omitempty" jsonschema:"only alerts of these apps"`
	Nodes      []string          `json:"nodes,omitempty" jsonschema:"only alerts of apps placed on these nodes (name or ID)"`
	Kinds      []string          `json:"kinds,omitempty" jsonschema:"only these rule kinds, e.g. threshold, crashloop"`
	Severities []string          `json:"severities,omitempty" jsonschema:"only info, warning or critical alerts"`
	Labels     map[string]string `json:"labels,omitempty" jsonschema:"only rules carrying all of these labels"`
	Duration   string            `json:"duration" jsonschema:"how long to silence, e.g. 1h, 4h, 24h (at most 90 days)"`
	Reason     string            `json:"reason,omitempty" jsonschema:"why the alerts are silenced"`
}

type silenceAlertRuleInput struct {
	App      string `json:"app" jsonschema:"the app that owns the rule"`
	RuleID   string `json:"rule_id" jsonschema:"the alert rule to silence"`
	Duration string `json:"duration" jsonschema:"how long to silence, e.g. 1h, 4h, 24h"`
	Reason   string `json:"reason,omitempty" jsonschema:"why the rule is silenced"`
}

type expireSilenceInput struct {
	ID string `json:"id" jsonschema:"the silence ID to end now"`
}

type alertHistoryInput struct {
	App     string `json:"app,omitempty" jsonschema:"only this app"`
	RuleID  string `json:"rule_id,omitempty" jsonschema:"only this rule"`
	Outcome string `json:"outcome,omitempty" jsonschema:"sent, silenced, grouped, inhibited, failed, ratelimited, flapping or skipped"`
	Event   string `json:"event,omitempty" jsonschema:"fired, resolved, flapping or flap_ended"`
	Since   string `json:"since,omitempty" jsonschema:"RFC 3339 lower bound"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum entries, newest first (default 50)"`
}

func registerAlertNoiseTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_alert_silences",
		Description: "List alert silences (active and pending; expired ones with include_expired). A silence keeps matching alerts evaluating and recorded in history but stops them notifying.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listSilencesInput) (*mcp.CallToolResult, []apiclient.SilenceResource, error) {
		out, err := client.ListAlertSilences(ctx, in.IncludeExpired)
		if err != nil {
			return nil, nil, fmt.Errorf("list alert silences: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "create_alert_silence",
		Description: "Silence alerts matching every given filter for a duration. At least one filter is required. Alerts still evaluate and appear in alert history as silenced; only notifications stop. Undo with expire_alert_silence.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createSilenceInput) (*mcp.CallToolResult, apiclient.SilenceResource, error) {
		m := apiclient.SilenceMatchers{RuleIDs: in.RuleIDs, Apps: in.Apps, Nodes: in.Nodes, Kinds: in.Kinds, Severities: in.Severities, Labels: in.Labels}
		out, err := client.CreateAlertSilence(ctx, apiclient.CreateSilenceRequest{Matchers: m, Duration: in.Duration, Reason: in.Reason})
		if err != nil {
			return nil, apiclient.SilenceResource{}, fmt.Errorf("create alert silence: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "silence_alert_rule",
		Description: "Quick action: silence one alert rule of an app for a duration such as 1h, 4h or 24h. Undo with expire_alert_silence.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in silenceAlertRuleInput) (*mcp.CallToolResult, apiclient.SilenceResource, error) {
		out, err := client.SilenceAlertRule(ctx, in.App, in.RuleID, in.Duration, in.Reason)
		if err != nil {
			return nil, apiclient.SilenceResource{}, fmt.Errorf("silence alert rule %q of app %q: %w", in.RuleID, in.App, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "expire_alert_silence",
		Description: "End a silence now so matching alerts notify again. The silence stays in history.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in expireSilenceInput) (*mcp.CallToolResult, apiclient.SilenceResource, error) {
		out, err := client.ExpireAlertSilence(ctx, in.ID)
		if err != nil {
			return nil, apiclient.SilenceResource{}, fmt.Errorf("expire alert silence %q: %w", in.ID, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_maintenance_windows",
		Description: "List recurring alert maintenance windows (cron plus duration plus timezone), with whether each is active now and its next start.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.MaintenanceWindowResource, error) {
		out, err := client.ListMaintenanceWindows(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list maintenance windows: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_alert_history",
		Description: "List alert firings and resolutions, newest first, with what happened to each notification (sent, silenced, grouped, inhibited, failed, ratelimited, flapping, skipped).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in alertHistoryInput) (*mcp.CallToolResult, []apiclient.AlertHistoryEntry, error) {
		q := apiclient.AlertHistoryQuery{App: in.App, RuleID: in.RuleID, Outcome: in.Outcome, Event: in.Event, Since: in.Since, Limit: in.Limit}
		out, err := client.ListAlertHistory(ctx, q)
		if err != nil {
			return nil, nil, fmt.Errorf("list alert history: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_status_page",
		Description: "Get the public status page settings and a preview of what it currently shows (component names, statuses, uptime). Read only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, statusPageOutput, error) {
		settings, err := client.GetStatusPage(ctx)
		if err != nil {
			return nil, statusPageOutput{}, fmt.Errorf("get status page: %w", err)
		}
		preview, err := client.StatusPagePreview(ctx)
		if err != nil {
			return nil, statusPageOutput{}, fmt.Errorf("preview status page: %w", err)
		}
		return nil, statusPageOutput{Settings: settings, Preview: preview}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_status_incidents",
		Description: "List operator-authored status page incidents and maintenance announcements with their updates. Read only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.StatusIncident, error) {
		out, err := client.ListStatusIncidents(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list status incidents: %w", err)
		}
		return nil, out, nil
	})
}

type statusPageOutput struct {
	Settings apiclient.StatusPageSettings `json:"settings"`
	Preview  apiclient.StatusPageView     `json:"preview"`
}

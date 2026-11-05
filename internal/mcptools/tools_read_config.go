package mcptools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type monitorResultsInput struct {
	ID     string `json:"id" jsonschema:"monitor ID"`
	Window string `json:"window,omitempty" jsonschema:"e.g. 1h, 24h, 7d"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max results"`
}

type policySummary struct {
	Scope   string            `json:"scope"`
	ScopeID string            `json:"scope_id"`
	Rules   map[string]string `json:"rules"`
}

type monitorSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Interval int    `json:"interval_seconds"`
	Enabled  bool   `json:"enabled"`
}

type monitorHealth struct {
	MonitorID string `json:"monitor_id"`
	Checks    int    `json:"checks"`
	Failed    int    `json:"failed"`
	LastOK    bool   `json:"last_ok"`
	LastAt    int64  `json:"last_at"`
	LastMS    int    `json:"last_latency_ms"`
	LastCode  int    `json:"last_code,omitempty"`
	LastNote  string `json:"last_reason,omitempty"`
}

func summarizeRule(r apiclient.DeployPolicyRule) string {
	s := r.Mode
	if len(r.Values) > 0 {
		s += ":" + strings.Join(r.Values, ",")
	}
	if r.Limit != 0 {
		s += ":" + strconv.FormatFloat(r.Limit, 'f', -1, 64)
	}
	return s
}

func monitorTarget(m apiclient.Monitor) string {
	if m.URL != "" {
		return m.URL
	}
	if m.Port != 0 {
		return m.Host + ":" + strconv.Itoa(m.Port)
	}
	return m.Host
}

// Read-only views of what the dashboard shows. Each calls the same REST route the
// dashboard uses, so abilities, org scoping and redaction stay server-side.
func registerReadConfigTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_deploy_policies",
		Description: "List deploy policies and their rules (mode, values, limit).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []policySummary, error) {
		pols, err := client.ListDeployPolicies(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploy policies: %w", err)
		}
		out := make([]policySummary, 0, len(pols))
		for _, p := range pols {
			rules := make(map[string]string, len(p.Rules))
			for k, r := range p.Rules {
				rules[k] = summarizeRule(r)
			}
			out = append(out, policySummary{Scope: p.Scope, ScopeID: p.ScopeID, Rules: rules})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_monitors",
		Description: "List uptime monitors with target and interval.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []monitorSummary, error) {
		mons, err := client.ListMonitors(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list monitors: %w", err)
		}
		out := make([]monitorSummary, 0, len(mons))
		for _, m := range mons {
			out = append(out, monitorSummary{ID: m.ID, Name: m.Name, Kind: m.Kind, Target: monitorTarget(m), Interval: m.IntervalSeconds, Enabled: m.Enabled == nil || *m.Enabled})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_monitor_results",
		Description: "Get a monitor's health: check and failure counts and the latest probe.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in monitorResultsInput) (*mcp.CallToolResult, monitorHealth, error) {
		res, err := client.MonitorResultsFor(ctx, in.ID, in.Window, in.Limit)
		if err != nil {
			return nil, monitorHealth{}, fmt.Errorf("get monitor results %q: %w", in.ID, err)
		}
		h := monitorHealth{MonitorID: res.MonitorID}
		for _, b := range res.Rollup {
			h.Checks += b.Total
			h.Failed += b.Failed
		}
		if n := len(res.Raw); n > 0 {
			last := res.Raw[0]
			for _, r := range res.Raw {
				if r.At > last.At {
					last = r
				}
			}
			h.LastOK, h.LastAt, h.LastMS, h.LastCode, h.LastNote = last.OK, last.At, last.LatencyMS, last.Code, last.Reason
		}
		return nil, h, nil
	})
}

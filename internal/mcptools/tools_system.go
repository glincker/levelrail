package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSystemTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_system_doctor",
		Description: "Local preflight: Docker, disk, ingress ports, database. Read only; same report as 'levelrail-cli doctor'.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.SystemDoctorResource, error) {
		report, err := client.GetSystemDoctor(ctx)
		if err != nil {
			return nil, apiclient.SystemDoctorResource{}, fmt.Errorf("get system doctor report: %w", err)
		}
		return nil, report, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_system_status",
		Description: "Control plane configured signals: secrets, telemetry, alerts, Docker reachability, data disk usage. Lighter than get_system_doctor.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.SystemStatusResource, error) {
		status, err := client.GetSystemStatus(ctx)
		if err != nil {
			return nil, apiclient.SystemStatusResource{}, fmt.Errorf("get system status: %w", err)
		}
		return nil, status, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_onboarding_status",
		Description: "Get whether the control plane's first-run onboarding flow has been completed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.OnboardingStateResource, error) {
		state, err := client.GetOnboardingState(ctx)
		if err != nil {
			return nil, apiclient.OnboardingStateResource{}, fmt.Errorf("get onboarding status: %w", err)
		}
		return nil, state, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_firewall_exposure",
		Description: "Published container ports reachable from outside, per node, with severity. Docker bypasses ufw for these. Read-only; no MCP tool can apply a rule.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, exposureSummary, error) {
		report, err := client.GetExposure(ctx, "", false)
		if err != nil {
			return nil, exposureSummary{}, fmt.Errorf("get firewall exposure: %w", err)
		}
		return nil, summarizeExposure(report), nil
	})

	addTool(server, &mcp.Tool{
		Name:        "prune_system",
		Description: "Remove every stopped container, dangling image, and unused volume or build cache the reconciler's current desired state doesn't need, fleet-wide. A routine day-2 cleanup action, same one 'levelrail-cli system-prune' runs; never touches a container, image, or volume any app or database still desires. Mutating.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.SystemPruneResult, error) {
		result, err := client.PruneSystem(ctx)
		if err != nil {
			return nil, apiclient.SystemPruneResult{}, fmt.Errorf("prune system: %w", err)
		}
		return nil, result, nil
	})
}

type exposureSummaryItem struct {
	Node        string `json:"node"`
	Container   string `json:"container"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Class       string `json:"class"`
	Severity    string `json:"severity"`
	Explanation string `json:"explanation"`
}

type exposureSummary struct {
	Exposed int                   `json:"exposed"`
	Items   []exposureSummaryItem `json:"items"`
}

func summarizeExposure(r apiclient.ExposureReport) exposureSummary {
	out := exposureSummary{Exposed: r.Exposed, Items: []exposureSummaryItem{}}
	for _, n := range r.Nodes {
		for _, f := range n.Findings {
			if f.Class == "exposed" || f.Class == "unknown" {
				out.Items = append(out.Items, exposureSummaryItem{n.NodeName, f.Container, f.HostPort, f.Protocol, f.Class, f.Severity, f.Explanation})
			}
		}
	}
	return out
}

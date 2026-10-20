package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxReachFindings = 50

type reachFindingBrief struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Subject  string `json:"subject"`
	Node     string `json:"node,omitempty"`
	Port     int    `json:"port,omitempty"`
}

type securityReachOutput struct {
	Counts          apiclient.ReachCounts `json:"counts"`
	RulesReadable   bool                  `json:"rules_readable"`
	RestrictedPorts int                   `json:"restricted_ports"`
	Findings        []reachFindingBrief   `json:"findings"`
	Truncated       bool                  `json:"truncated,omitempty"`
}

type exportDestinationBrief struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Enabled       bool   `json:"enabled"`
	Behind        bool   `json:"behind"`
	CircuitOpen   bool   `json:"circuit_open"`
	QueueDepth    int64  `json:"queue_depth"`
	Dropped       int64  `json:"dropped"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

type exportDestinationsOutput struct {
	Destinations []exportDestinationBrief `json:"destinations"`
}

type templateRequirementsOutput struct {
	ID          string                         `json:"id"`
	Level       string                         `json:"level"`
	Findings    []apiclient.TemplateFinding    `json:"findings"`
	Protections []apiclient.TemplateProtection `json:"protections"`
}

func registerSecurityTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_security_reachability",
		Description: "What is reachable from outside, with severity counts and findings. Root only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, securityReachOutput, error) {
		rep, err := client.GetSecurityReachability(ctx, false)
		if err != nil {
			return nil, securityReachOutput{}, fmt.Errorf("get security reachability: %w", err)
		}
		out := securityReachOutput{Counts: rep.Counts, RulesReadable: rep.RulesReadable, RestrictedPorts: rep.RestrictedPorts, Findings: []reachFindingBrief{}}
		for i, f := range rep.Findings {
			if i == maxReachFindings {
				out.Truncated = true
				break
			}
			out.Findings = append(out.Findings, reachFindingBrief{Kind: f.Kind, Severity: f.Severity, Subject: f.Subject.Type + "/" + f.Subject.Name, Node: f.Node, Port: f.Port})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_export_destinations",
		Description: "Data export destinations with delivery health and whether each is behind. Root only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, exportDestinationsOutput, error) {
		list, err := client.ListExportDestinations(ctx)
		if err != nil {
			return nil, exportDestinationsOutput{}, fmt.Errorf("list export destinations: %w", err)
		}
		out := exportDestinationsOutput{Destinations: make([]exportDestinationBrief, 0, len(list))}
		for _, d := range list {
			s := d.Status
			out.Destinations = append(out.Destinations, exportDestinationBrief{
				ID: d.ID, Name: d.Name, Kind: d.Kind, Enabled: d.Enabled,
				Behind:      d.Enabled && (s.CircuitOpen || s.LastErrorAt > s.LastSuccessAt),
				CircuitOpen: s.CircuitOpen, QueueDepth: s.QueueDepth, Dropped: s.DroppedTotal,
				LastSuccessAt: s.LastSuccessAt, LastError: s.LastError,
			})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_template_requirements",
		Description: "A service template's security level, findings and built-in protections.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in serviceTemplateIDInput) (*mcp.CallToolResult, templateRequirementsOutput, error) {
		t, err := client.GetServiceTemplate(ctx, in.ID)
		if err != nil {
			return nil, templateRequirementsOutput{}, fmt.Errorf("get template requirements %q: %w", in.ID, err)
		}
		r := t.Requirements
		out := templateRequirementsOutput{ID: t.ID, Level: r.Level, Findings: r.Findings, Protections: r.Protections}
		if out.Findings == nil {
			out.Findings = []apiclient.TemplateFinding{}
		}
		if out.Protections == nil {
			out.Protections = []apiclient.TemplateProtection{}
		}
		return nil, out, nil
	})
}

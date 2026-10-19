package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type inboxInput struct {
	UnreadOnly bool `json:"unread_only,omitempty"`
	Limit      int  `json:"limit,omitempty"`
}

type inboxOutput struct {
	Unread int                   `json:"unread"`
	Items  []apiclient.InboxItem `json:"items"`
}

type simulateInput struct {
	CPUMilli  int64  `json:"cpu_milli,omitempty"`
	MemoryMiB int64  `json:"memory_mib,omitempty"`
	Count     int    `json:"count,omitempty"`
	Selector  string `json:"selector,omitempty"`
	Zone      string `json:"zone,omitempty"`
	Node      string `json:"node,omitempty"`
}

type simulateOutput struct {
	Placed    []apiclient.SimulatedPlacement `json:"placed"`
	Unplaced  int                            `json:"unplaced"`
	Displaced int                            `json:"displaced,omitempty"`
	Lost      []string                       `json:"lost_node_ids,omitempty"`
}

type cdnOverviewInput struct {
	Window string `json:"window,omitempty" jsonschema:"5m, 1h or 24h"`
}

func registerInterfaceExtraTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_inbox",
		Description: "The caller's notification inbox, newest first, with the unread count.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in inboxInput) (*mcp.CallToolResult, inboxOutput, error) {
		items, err := client.ListInbox(ctx, in.UnreadOnly, in.Limit)
		if err != nil {
			return nil, inboxOutput{}, fmt.Errorf("list inbox: %w", err)
		}
		unread, err := client.InboxUnreadCount(ctx)
		if err != nil {
			return nil, inboxOutput{}, fmt.Errorf("inbox unread count: %w", err)
		}
		return nil, inboxOutput{Unread: unread, Items: items}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "simulate_placement",
		Description: "Dry run, root token. Without zone or node: where count apps of cpu_milli and memory_mib would land. With exactly one of zone or node: what happens if it is lost.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in simulateInput) (*mcp.CallToolResult, simulateOutput, error) {
		if in.Zone != "" || in.Node != "" {
			out, err := client.SimulateFailure(ctx, in.Zone, in.Node)
			if err != nil {
				return nil, simulateOutput{}, fmt.Errorf("simulate node failure: %w", err)
			}
			return nil, simulateOutput{Placed: out.Placed, Unplaced: out.Unplaced, Displaced: out.Count, Lost: out.Lost}, nil
		}
		out, err := client.SimulatePlacement(ctx, in.CPUMilli, in.MemoryMiB, max(in.Count, 1), in.Selector)
		if err != nil {
			return nil, simulateOutput{}, fmt.Errorf("simulate placement: %w", err)
		}
		return nil, simulateOutput{Placed: out.Placed, Unplaced: out.Unplaced}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_cdn_overview",
		Description: "Fleet edge view: request rate, 5xx rate, cache hit ratio and per node traffic.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cdnOverviewInput) (*mcp.CallToolResult, apiclient.CDNOverview, error) {
		out, err := client.GetCDNOverview(ctx, in.Window)
		if err != nil {
			return nil, apiclient.CDNOverview{}, fmt.Errorf("get cdn overview: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "verify_audit_log",
		Description: "Walk the audit hash chain and report edited, removed or truncated rows. Root token.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.AuditChainReport, error) {
		out, err := client.VerifyAuditLog(ctx)
		if err != nil {
			return nil, apiclient.AuditChainReport{}, fmt.Errorf("verify audit log: %w", err)
		}
		return nil, out, nil
	})
}

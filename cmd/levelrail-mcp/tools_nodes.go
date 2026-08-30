package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerNodeTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_nodes",
		Description: "List every node enrolled in the control plane: id, name, address, status, schedulable/cordon state, and which workload kinds (app, build) it accepts. Useful for diagnosing which machine an app runs on.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.NodeResource, error) {
		nodes, err := client.ListNodes(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list nodes: %w", err)
		}
		return nil, nodes, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_node",
		Description: "Get one node's current state: address, status, schedulable/cordon state, accepted workload kinds, join/last-seen timestamps.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in nodeIDInput) (*mcp.CallToolResult, apiclient.NodeResource, error) {
		node, err := client.GetNode(ctx, in.ID)
		if err != nil {
			return nil, apiclient.NodeResource{}, fmt.Errorf("get node %q: %w", in.ID, err)
		}
		return nil, node, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_node_health",
		Description: "Get one node's current stored reconcile conditions (type, status, reason, message) from the node health controller. This is current status, not a historical log.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in nodeIDInput) (*mcp.CallToolResult, []apiclient.ConditionResource, error) {
		conditions, err := client.GetNodeHealth(ctx, in.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("get health for node %q: %w", in.ID, err)
		}
		return nil, conditions, nil
	})
}

type nodeIDInput struct {
	ID string `json:"id" jsonschema:"the node's id, from list_nodes"`
}

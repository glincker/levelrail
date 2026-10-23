package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type canaryStartInput struct {
	Name         string  `json:"name" jsonschema:"the app's name"`
	Image        string  `json:"image" jsonschema:"image reference to run as the canary"`
	Weight       int     `json:"weight,omitempty" jsonschema:"manual canary only: percent of traffic, 1-99. Omit when using stages"`
	Stages       []int   `json:"stages,omitempty" jsonschema:"staged canary: ascending traffic percents such as [10,25,50,100]; the control plane advances them and rolls back on a breach"`
	Interval     string  `json:"interval,omitempty" jsonschema:"staged canary: how long each stage observes traffic, such as 5m"`
	MaxErrorRate float64 `json:"max_error_rate,omitempty" jsonschema:"staged canary: roll back when the canary 5xx rate exceeds the stable release's by this fraction, 0.02 is two points"`
	MaxP95Ratio  float64 `json:"max_p95_ratio,omitempty" jsonschema:"staged canary: roll back when the canary p95 latency exceeds this multiple of the stable release's"`
}

func registerCanaryTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_canary",
		Description: "An app's canary: image, traffic share, stages, live error rate and p95 against stable, and how the last one ended.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.CanaryResource, error) {
		out, err := client.GetCanary(ctx, in.Name)
		if err != nil {
			return nil, apiclient.CanaryResource{}, fmt.Errorf("get canary for app %q: %w", in.Name, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "start_canary",
		Description: "Run a new image beside an app with a share of traffic. With stages the control plane steps the share up and rolls back on an error rate or p95 breach against stable; without stages it is a manual canary at weight. One per app, single-replica image apps only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in canaryStartInput) (*mcp.CallToolResult, apiclient.CanaryResource, error) {
		out, err := client.StartCanaryWith(ctx, in.Name, apiclient.CanaryStartRequest{
			Image: in.Image, Weight: in.Weight, Stages: in.Stages, Interval: in.Interval,
			MaxErrorRate: in.MaxErrorRate, MaxP95Ratio: in.MaxP95Ratio,
		})
		if err != nil {
			return nil, apiclient.CanaryResource{}, fmt.Errorf("start canary for app %q: %w", in.Name, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "promote_canary",
		Description: "Promote now: deploy the canary image to the app through the normal deploy path, skipping any remaining stages, then remove the canary.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.CanaryResource, error) {
		out, err := client.PromoteCanary(ctx, in.Name)
		if err != nil {
			return nil, apiclient.CanaryResource{}, fmt.Errorf("promote canary for app %q: %w", in.Name, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "abort_canary",
		Description: "Abort the canary: remove it so all traffic returns to the app's current release.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.CanaryResource, error) {
		out, err := client.AbortCanary(ctx, in.Name)
		if err != nil {
			return nil, apiclient.CanaryResource{}, fmt.Errorf("abort canary for app %q: %w", in.Name, err)
		}
		return nil, out, nil
	})
}

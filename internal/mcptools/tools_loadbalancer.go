package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type setLoadBalancerInput struct {
	Name   string                       `json:"name" jsonschema:"the app's name"`
	Config apiclient.LoadBalancerConfig `json:"config" jsonschema:"the complete load balancer config, replacing any existing one. algorithm is one of round_robin, least_conn, ip_hash, uri_hash, cookie, weighted; durations are strings like 5s"`
}

type exportLoadBalancerInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Format string `json:"format" jsonschema:"terraform, cdk, cloudformation, caddy or caddy-json"`
}

func registerLoadBalancerTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_load_balancer",
		Description: "Get an app's load balancer config: algorithm, weights, health checks, retries, drain and slow start, rate limit, upstream TLS. Unconfigured means a single upstream. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LoadBalancerResource, error) {
		res, err := client.GetLoadBalancer(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LoadBalancerResource{}, fmt.Errorf("get load balancer for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_load_balancer_status",
		Description: "Get the live upstream table for an app's load balancer: each replica's state (healthy, unhealthy, draining), weight, active requests, failures and last health check, plus the reconciler's reason string. Use it to see why traffic is not reaching a replica. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LoadBalancerStatus, error) {
		st, err := client.GetLoadBalancerStatus(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LoadBalancerStatus{}, fmt.Errorf("get load balancer status for app %q: %w", in.Name, err)
		}
		return nil, st, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_app_load_balancer",
		Description: "Create or replace an app's load balancer config. The reconciler applies it on its next pass; only affects routing of the app's domains. Rejected with a list of problems if the config is invalid.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setLoadBalancerInput) (*mcp.CallToolResult, apiclient.LoadBalancerResource, error) {
		res, err := client.SetLoadBalancer(ctx, in.Name, in.Config)
		if err != nil {
			return nil, apiclient.LoadBalancerResource{}, fmt.Errorf("set load balancer for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "clear_app_load_balancer",
		Description: "Remove an app's load balancer so its domains route to a single upstream again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, struct{ Cleared bool }, error) {
		if err := client.DeleteLoadBalancer(ctx, in.Name); err != nil {
			return nil, struct{ Cleared bool }{}, fmt.Errorf("clear load balancer for app %q: %w", in.Name, err)
		}
		return nil, struct{ Cleared bool }{Cleared: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "export_app_load_balancer",
		Description: "Generate an infrastructure-as-code definition of an app's load balancer: terraform (AWS ALB HCL), cdk (AWS CDK TypeScript), cloudformation (YAML), caddy (Caddyfile) or caddy-json. Pure text generation, calls no cloud API. Warnings list settings with no equivalent in the target.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in exportLoadBalancerInput) (*mcp.CallToolResult, apiclient.LoadBalancerArtifact, error) {
		art, err := client.ExportLoadBalancer(ctx, in.Name, in.Format)
		if err != nil {
			return nil, apiclient.LoadBalancerArtifact{}, fmt.Errorf("export load balancer for app %q: %w", in.Name, err)
		}
		return nil, art, nil
	})
}

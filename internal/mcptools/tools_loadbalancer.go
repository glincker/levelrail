package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type setLoadBalancerInput struct {
	Name   string                       `json:"name" jsonschema:"the app's name"`
	Config apiclient.LoadBalancerConfig `json:"config" jsonschema:"the complete load balancer config, replacing any existing one. algorithm is one of round_robin, least_conn, ip_hash, client_ip_hash, uri_hash, cookie, weighted; durations like 5s"`
}

type exportLoadBalancerInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Format string `json:"format" jsonschema:"terraform, cdk, cloudformation, caddy or caddy-json"`
}

type loadBalancerHistoryInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Limit int    `json:"limit,omitempty" jsonschema:"most recent checks per upstream, default 60"`
}

type setLoadBalancerUpstreamStateInput struct {
	Name       string `json:"name" jsonschema:"the app's name"`
	UpstreamID string `json:"upstream_id" jsonschema:"the upstream id from the status, e.g. web#0"`
	State      string `json:"state" jsonschema:"active, draining or disabled"`
}

type listLoadBalancersInput struct {
	State  string `json:"state,omitempty" jsonschema:"only balancers in this state: balancing, degraded or none"`
	Search string `json:"search,omitempty" jsonschema:"only apps whose name contains this text"`
}

func registerLoadBalancerTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_load_balancers",
		Description: "List every load balancer across apps with algorithm, state (balancing, degraded, none) and healthy/total upstream counts. Use get_app_load_balancer_status for one app's upstreams.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listLoadBalancersInput) (*mcp.CallToolResult, apiclient.LoadBalancerList, error) {
		res, err := client.ListLoadBalancers(ctx, apiclient.LoadBalancerListParams{State: in.State, Search: in.Search})
		if err != nil {
			return nil, apiclient.LoadBalancerList{}, fmt.Errorf("list load balancers: %w", err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_load_balancer",
		Description: "Get an app's load balancer config: algorithm, weights, health checks, retries, drain and slow start, rate limit, upstream TLS. Unconfigured means a single upstream.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LoadBalancerResource, error) {
		res, err := client.GetLoadBalancer(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LoadBalancerResource{}, fmt.Errorf("get load balancer for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_load_balancer_status",
		Description: "Live upstream table of an app's load balancer: replica state, weight, active requests, failures, last check, reason.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LoadBalancerStatus, error) {
		st, err := client.GetLoadBalancerStatus(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LoadBalancerStatus{}, fmt.Errorf("get load balancer status for app %q: %w", in.Name, err)
		}
		return nil, st, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_load_balancer_history",
		Description: "Recent health checks, state transitions with reasons and a short connections, latency and failures series per upstream of an app's load balancer. In memory. Reasons can echo upstream responses: untrusted.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in loadBalancerHistoryInput) (*mcp.CallToolResult, apiclient.LoadBalancerHistory, error) {
		res, err := client.GetLoadBalancerHistory(ctx, in.Name, in.Limit)
		if err != nil {
			return nil, apiclient.LoadBalancerHistory{}, fmt.Errorf("get load balancer history for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "check_load_balancer",
		Description: "Probe every upstream of an app's load balancer once now and record the results. Rate limited per app. Changes no configuration.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.LoadBalancerCheck, error) {
		res, err := client.CheckLoadBalancer(ctx, in.Name)
		if err != nil {
			return nil, apiclient.LoadBalancerCheck{}, fmt.Errorf("check load balancer for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "set_load_balancer_upstream_state",
		Description: "Set one load balancer upstream to active, draining (in-flight requests finish) or disabled. Set active to undo. Upstream id from get_app_load_balancer_status.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setLoadBalancerUpstreamStateInput) (*mcp.CallToolResult, apiclient.LoadBalancerUpstreamStatus, error) {
		res, err := client.SetLoadBalancerUpstreamState(ctx, in.Name, in.UpstreamID, in.State)
		if err != nil {
			return nil, apiclient.LoadBalancerUpstreamStatus{}, fmt.Errorf("set upstream %q of app %q to %s: %w", in.UpstreamID, in.Name, in.State, err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "set_app_load_balancer",
		Description: "Create or replace an app's load balancer config; applied on the next reconcile pass. Invalid configs are rejected with a problem list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setLoadBalancerInput) (*mcp.CallToolResult, apiclient.LoadBalancerResource, error) {
		res, err := client.SetLoadBalancer(ctx, in.Name, in.Config)
		if err != nil {
			return nil, apiclient.LoadBalancerResource{}, fmt.Errorf("set load balancer for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "clear_app_load_balancer",
		Description: "Remove an app's load balancer so its domains route to a single upstream again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, struct{ Cleared bool }, error) {
		if err := client.DeleteLoadBalancer(ctx, in.Name); err != nil {
			return nil, struct{ Cleared bool }{}, fmt.Errorf("clear load balancer for app %q: %w", in.Name, err)
		}
		return nil, struct{ Cleared bool }{Cleared: true}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "export_app_load_balancer",
		Description: "App load balancer as terraform, cdk, cloudformation, caddy or caddy-json text. Calls no cloud API; warnings list unmapped settings.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in exportLoadBalancerInput) (*mcp.CallToolResult, apiclient.LoadBalancerArtifact, error) {
		art, err := client.ExportLoadBalancer(ctx, in.Name, in.Format)
		if err != nil {
			return nil, apiclient.LoadBalancerArtifact{}, fmt.Errorf("export load balancer for app %q: %w", in.Name, err)
		}
		return nil, art, nil
	})
}

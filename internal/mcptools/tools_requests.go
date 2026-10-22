package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAppRequestsTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_requests",
		Description: "App HTTP health at the ingress: request rate, 4xx and 5xx rates, p50/p95/p99. Tells whether users are affected or a deploy changed latency.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appRequestsInput) (*mcp.CallToolResult, apiclient.AppRequestsResource, error) {
		window := defaultMetricsWindow
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, apiclient.AppRequestsResource{}, fmt.Errorf("get requests for app %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			window = d
		}
		var step time.Duration
		if in.Step != "" {
			d, err := time.ParseDuration(in.Step)
			if err != nil {
				return nil, apiclient.AppRequestsResource{}, fmt.Errorf("get requests for app %q: invalid step %q: %w", in.Name, in.Step, err)
			}
			step = d
		}
		to := time.Now()
		res, err := client.QueryAppRequests(ctx, in.Name, to.Add(-window), to, step)
		if err != nil {
			return nil, apiclient.AppRequestsResource{}, fmt.Errorf("get requests for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})
}

func registerAppTrafficTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_traffic",
		Description: "App ingress traffic: rates, p50/p95/p99, replicas, routes, 5xx spikes by deploy, referrers, countries, samples, SLO burn.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appTrafficInput) (*mcp.CallToolResult, apiclient.AppTrafficResource, error) {
		window := in.Window
		if window == "" {
			window = "1h"
		}
		res, err := client.GetAppTraffic(ctx, in.Name, window)
		if err != nil {
			return nil, apiclient.AppTrafficResource{}, fmt.Errorf("get traffic for app %q: %w", in.Name, err)
		}
		return nil, res, nil
	})
}

type appTrafficInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Window string `json:"window,omitempty" jsonschema:"one of 5m, 1h, 24h or 7d; default 1h"`
}

type appRequestsInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Since string `json:"since,omitempty" jsonschema:"how far back to query, e.g. '1h', '30m'; default 1h"`
	Step  string `json:"step,omitempty" jsonschema:"bucket width, e.g. '1m'; omitted lets the server choose"`
}

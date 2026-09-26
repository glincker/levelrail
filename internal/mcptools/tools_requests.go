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
		Description: "Get an app's HTTP request health measured at the ingress with no app changes: request rate, 4xx and 5xx error rates, latency p50/p95/p99, plus a summary for the window. Use it to tell whether users are affected and whether a deploy changed latency or errors.",
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

type appRequestsInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Since string `json:"since,omitempty" jsonschema:"how far back to query, e.g. '1h', '30m'; default 1h"`
	Step  string `json:"step,omitempty" jsonschema:"bucket width, e.g. '1m'; omitted lets the server choose"`
}

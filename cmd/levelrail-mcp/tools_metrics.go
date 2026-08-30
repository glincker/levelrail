package main

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultMetricsWindow mirrors internal/api/metrics.go's own
// defaultQueryWindow: what get_app_metrics queries when the caller
// doesn't set since.
const defaultMetricsWindow = time.Hour

func registerAppMetricsTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_metrics",
		Description: "Query an app's time-series resource metrics (e.g. cpu_percent, memory_usage_bytes, memory_limit_bytes, network_rx_bytes, network_tx_bytes, disk_read_bytes, disk_write_bytes) over a time window, aggregated into buckets. This is how to answer 'why was this app slow/using too much memory at time X' without leaving the tool call.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appMetricsInput) (*mcp.CallToolResult, apiclient.AppMetricsResource, error) {
		window := defaultMetricsWindow
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, apiclient.AppMetricsResource{}, fmt.Errorf("get metrics for app %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			window = d
		}
		var step time.Duration
		if in.Step != "" {
			d, err := time.ParseDuration(in.Step)
			if err != nil {
				return nil, apiclient.AppMetricsResource{}, fmt.Errorf("get metrics for app %q: invalid step %q: %w", in.Name, in.Step, err)
			}
			step = d
		}
		to := time.Now()
		from := to.Add(-window)

		metrics, err := client.QueryAppMetrics(ctx, in.Name, in.Metric, from, to, step)
		if err != nil {
			return nil, apiclient.AppMetricsResource{}, fmt.Errorf("get metrics for app %q metric %q: %w", in.Name, in.Metric, err)
		}
		return nil, metrics, nil
	})
}

type appMetricsInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Metric string `json:"metric" jsonschema:"metric key to query, e.g. cpu_percent, memory_usage_bytes, memory_limit_bytes, network_rx_bytes, network_tx_bytes, disk_read_bytes, disk_write_bytes"`
	Since  string `json:"since,omitempty" jsonschema:"how far back to query, e.g. '1h', '30m'; default 1h"`
	Step   string `json:"step,omitempty" jsonschema:"aggregation bucket width, e.g. '60s'; omitted returns raw unaggregated samples"`
}

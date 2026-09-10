package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runAppsMetrics implements "apps metrics <name> --metric NAME": GET
// /api/v1/apps/{name}/metrics (internal/api/metrics.go's own
// handleQueryMetrics), the same query the dashboard's per-app metrics
// charts read (client.QueryAppMetrics). The server is the authority on
// which metric names are valid (cpu_percent, memory_usage_bytes,
// memory_limit_bytes, network_rx_bytes, network_tx_bytes,
// disk_read_bytes, disk_write_bytes are the ones collector.go writes
// today); this command does not maintain its own copy of that list. See
// runMetricsCommand (metrics_cmd.go) for the flag parsing and rendering
// shared with "databases metrics"/"nodes metrics".
func runAppsMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runMetricsCommand(prog, args, stdout, stderr, lookupEnv, metricsCommandConfig{
		cmdLabel: "apps metrics",
		argLabel: "app name",
		errNoun:  "app",
		usage:    appsMetricsUsage,
		query: func(ctx context.Context, client *Client, name, metric string, from, to time.Time, step time.Duration) (metricsQueryResult, error) {
			res, err := client.QueryAppMetrics(ctx, name, metric, from, to, step)
			return metricsQueryResult{Value: res, Metric: res.Metric, Points: res.Points}, err
		},
	})
}

func appsMetricsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps metrics <name> --metric NAME [flags]

Queries one metric's time series for an app (the same data the dashboard's
per-app metrics chart reads), aggregated into buckets by --step or, if
omitted, returned as raw unaggregated samples.

Flags:
%[2]s`, prog, metricsUsageFlags(prog))
}

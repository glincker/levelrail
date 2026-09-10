package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runNodesMetrics implements "nodes metrics <id> --metric NAME": GET
// /api/v1/nodes/{id}/metrics (internal/api/node_metrics.go's own
// handleQueryNodeMetrics). Most metrics here are a sum across every
// service currently placed on the node, not a true host-level reading
// (see that handler's own doc comment); a few (disk usage, OS patches
// available) are real per-node readings instead. Either way, the server
// is the sole authority on which metric names it accepts. See
// runMetricsCommand (metrics_cmd.go) for the flag parsing and rendering
// shared with "apps metrics"/"databases metrics"; the resource_count
// line below is the one thing unique to this command.
func runNodesMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runMetricsCommand(prog, args, stdout, stderr, lookupEnv, metricsCommandConfig{
		cmdLabel: "nodes metrics",
		argLabel: "node id",
		errNoun:  "node",
		usage:    nodesMetricsUsage,
		query: func(ctx context.Context, client *Client, id, metric string, from, to time.Time, step time.Duration) (metricsQueryResult, error) {
			res, err := client.QueryNodeMetrics(ctx, id, metric, from, to, step)
			return metricsQueryResult{
				Value:  res,
				Metric: res.Metric,
				Points: res.Points,
				Extra:  func(w io.Writer) { _, _ = fmt.Fprintf(w, "resource_count: %d\n", res.ResourceCount) },
			}, err
		},
	})
}

func nodesMetricsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes metrics <id> --metric NAME [flags]

Queries one metric's time series for a node: a sum across every service
currently placed on it for most metric names, or a real per-node reading
for disk usage and available OS patches (see the dashboard's node metrics
view for the same data). resource_count in the output is how many placed
services actually contributed a sample, not how many are placed in total.

Flags:
%[2]s`, prog, metricsUsageFlags(prog))
}

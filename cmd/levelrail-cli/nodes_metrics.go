package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Most metrics sum the services placed on the node, not host readings; the server decides which names are valid.
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

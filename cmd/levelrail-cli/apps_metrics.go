package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// The server decides which metric names are valid, so the CLI keeps no list of them.
func runAppsMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if rest, wantRequests := splitRequestsFlag(args); wantRequests {
		return runAppsRequests(prog, rest, stdout, stderr, lookupEnv)
	}
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

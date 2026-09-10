package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runDatabasesMetrics implements "databases metrics <name> --metric
// NAME": GET /api/v1/databases/{name}/metrics
// (internal/api/database_metrics.go's handleQueryDatabaseMetrics), the
// database-kind counterpart to "apps metrics" (apps_metrics.go). See
// runMetricsCommand (metrics_cmd.go) for the flag parsing and rendering
// shared between the two.
func runDatabasesMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runMetricsCommand(prog, args, stdout, stderr, lookupEnv, metricsCommandConfig{
		cmdLabel: "databases metrics",
		argLabel: "database name",
		errNoun:  "database",
		usage:    databasesMetricsUsage,
		query: func(ctx context.Context, client *Client, name, metric string, from, to time.Time, step time.Duration) (metricsQueryResult, error) {
			res, err := client.QueryDatabaseMetrics(ctx, name, metric, from, to, step)
			return metricsQueryResult{Value: res, Metric: res.Metric, Points: res.Points}, err
		},
	})
}

func databasesMetricsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases metrics <name> --metric NAME [flags]

Queries one metric's time series for a database (the same data the
dashboard's per-database metrics chart reads), aggregated into buckets by
--step or, if omitted, returned as raw unaggregated samples.

Flags:
%[2]s`, prog, metricsUsageFlags(prog))
}

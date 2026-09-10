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
// database-kind counterpart to "apps metrics" (apps_metrics.go); same
// flags, same behavior, only the underlying client call and resource
// noun differ.
func runDatabasesMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases metrics", "print the queried points as JSON to stdout and nothing else", stderr)
	var metric, since, from, to, step string
	fs.StringVar(&metric, "metric", "", "metric name to query, e.g. cpu_percent, memory_usage_bytes (required)")
	fs.StringVar(&since, "since", "", "how far back to query, e.g. \"1h\", \"30m\" (default: 1h; mutually exclusive with --from)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the query window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the query window (default: now)")
	fs.StringVar(&step, "step", "", "aggregation bucket size, e.g. \"60s\" (default: raw unaggregated samples)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesMetricsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases metrics", "database name")
	if !ok {
		return exitUsage
	}

	if metric == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--metric is required"))
	}

	fromTime, toTime, err := resolveTimeRange(timeRangeFlags{since: since, from: from, to: to}, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	stepDuration, err := parseStepFlag(step)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.QueryDatabaseMetrics(context.Background(), name, metric, fromTime, toTime, stepDuration)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query metrics for database %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printMetricPointsHuman(stdout, result.Metric, result.Points) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func databasesMetricsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases metrics <name> --metric NAME [flags]

Queries one metric's time series for a database (the same data the
dashboard's per-database metrics chart reads), aggregated into buckets by
--step or, if omitted, returned as raw unaggregated samples.

Flags:
  --metric string          metric name to query, e.g. cpu_percent, memory_usage_bytes (required)
  --since string          how far back to query, e.g. "1h", "30m" (default: 1h)
  --from string            RFC3339 start of the query window (overrides --since)
  --to string                RFC3339 end of the query window (default: now)
  --step string            aggregation bucket size, e.g. "60s" (default: raw samples)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the queried points as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

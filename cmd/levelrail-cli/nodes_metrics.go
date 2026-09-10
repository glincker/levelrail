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
// is the sole authority on which metric names it accepts.
func runNodesMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes metrics", "print the queried points as JSON to stdout and nothing else", stderr)
	var metric, since, from, to, step string
	fs.StringVar(&metric, "metric", "", "metric name to query, e.g. cpu_percent, memory_usage_bytes (required)")
	fs.StringVar(&since, "since", "", "how far back to query, e.g. \"1h\", \"30m\" (default: 1h; mutually exclusive with --from)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the query window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the query window (default: now)")
	fs.StringVar(&step, "step", "", "aggregation bucket size, e.g. \"60s\" (default: raw unaggregated samples)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesMetricsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes metrics", "node id")
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

	result, err := client.QueryNodeMetrics(context.Background(), id, metric, fromTime, toTime, stepDuration)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query metrics for node %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = fmt.Fprintf(stdout, "resource_count: %d\n", result.ResourceCount)
		printMetricPointsHuman(stdout, result.Metric, result.Points)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
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

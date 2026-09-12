package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// metricsQueryResult is metricsCommandConfig.Query's return shape:
// Value is the full apiclient resource (what --json/--output json/
// --query render against, so a node's extra resource_count field still
// round-trips), Metric/Points are that same resource's own fields
// pulled out for printMetricPointsHuman's table, and Extra is an
// optional human-output line (nodes metrics' "resource_count: N") that
// apps/databases metrics have nothing to add, hence nil.
type metricsQueryResult struct {
	Value  any
	Metric string
	Points []metricPointResource
	Extra  func(io.Writer)
}

// metricsCommandConfig is runMetricsCommand's per-command input: the
// wording and the single apiclient call that differ between "apps
// metrics", "databases metrics", and "nodes metrics", which otherwise
// share every line of flag parsing, time-range resolution, and
// --output/--query rendering. See apps_deploy.go's own
// deployOrRollbackConfig for the same shared-implementation shape
// applied to a different pair of commands.
type metricsCommandConfig struct {
	cmdLabel string
	argLabel string
	errNoun  string
	usage    func(string) string
	query    func(ctx context.Context, client *Client, id, metric string, from, to time.Time, step time.Duration) (metricsQueryResult, error)
}

// runMetricsCommand is "apps metrics"/"databases metrics"/"nodes
// metrics"'s shared implementation.
func runMetricsCommand(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), cfg metricsCommandConfig) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cfg.cmdLabel, "print the queried points as JSON to stdout and nothing else", stderr)
	var metric, since, from, to, step string
	fs.StringVar(&metric, "metric", "", "metric name to query, e.g. cpu_percent, memory_usage_bytes (required)")
	fs.StringVar(&since, "since", "", "how far back to query, e.g. \"1h\", \"30m\" (default: 1h; mutually exclusive with --from)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the query window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the query window (default: now)")
	fs.StringVar(&step, "step", "", "aggregation bucket size, e.g. \"60s\" (default: raw unaggregated samples)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cfg.usage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, cfg.cmdLabel, cfg.argLabel)
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

	result, err := cfg.query(context.Background(), client, id, metric, fromTime, toTime, stepDuration)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query metrics for %s %q: %w", cfg.errNoun, id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result.Value, func() {
		if result.Extra != nil {
			result.Extra(stdout)
		}
		printMetricPointsHuman(stdout, result.Metric, result.Points)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// metricsUsageFlags is the closing --metric/--since/.../-h flags block
// shared verbatim by "apps metrics"/"databases metrics"/"nodes
// metrics"'s own usage text: the same kind of footer nearly every other
// apiFlagSet-based command's usage function already repeats on its own
// (see nodes_health.go's nodesHealthUsage for one of many precedents).
// Factored out here specifically because three near-identical copies of
// it landing in one PR is what pushes new-code duplication over
// SonarCloud's gate, not because the wider repo-wide pattern itself
// needs to change.
func metricsUsageFlags(prog string) string {
	return fmt.Sprintf(`  --metric string          metric name to query, e.g. cpu_percent, memory_usage_bytes (required)
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

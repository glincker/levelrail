package main

import (
	"context"
	"fmt"
	"io"
)

// runNodesCapacityForecast implements "nodes capacity-forecast <id>":
// GET /api/v1/nodes/{id}/capacity-forecast, a rough "days until full at
// the current trend" projection for this node's disk and memory, from a
// deterministic linear-trend fit over recent usage history (never an
// external model, never applied automatically).
func runNodesCapacityForecast(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes capacity-forecast", "print the capacity forecast as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesCapacityForecastUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "nodes capacity-forecast", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	forecast, err := client.GetNodeCapacityForecast(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get capacity forecast for node %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, forecast, func() {
		printCapacityForecastHuman(stdout, forecast)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// printCapacityForecastHuman renders a nodeCapacityForecastResource the
// way an operator reads it at a glance: one line per resource that
// actually has a growing trend, "no concerning trend" when neither does
// (the same "absence means nothing to warn about" contract the API
// response itself commits to), and the honesty disclaimer last so it
// doesn't get lost above the numbers.
func printCapacityForecastHuman(out io.Writer, f nodeCapacityForecastResource) {
	_, _ = fmt.Fprintf(out, "Lookback window: %s\n", f.LookbackWindow)
	if f.Disk == nil && f.Memory == nil {
		_, _ = fmt.Fprintln(out, "No concerning disk or memory trend over the lookback window.")
		_, _ = fmt.Fprintln(out, f.Note)
		return
	}
	if f.Disk != nil {
		_, _ = fmt.Fprintf(out, "Disk:   %s\n", formatCapacityForecastMetric(*f.Disk))
	}
	if f.Memory != nil {
		_, _ = fmt.Fprintf(out, "Memory: %s\n", formatCapacityForecastMetric(*f.Memory))
	}
	_, _ = fmt.Fprintln(out, f.Note)
}

func formatCapacityForecastMetric(m capacityForecastMetric) string {
	return fmt.Sprintf(
		"%s of %s used, growing ~%s/day, ~%.1f days until full (projected %s)",
		formatRecommendationBytes(int64(m.CurrentUsedBytes)),
		formatRecommendationBytes(int64(m.TotalBytes)),
		formatRecommendationBytes(int64(m.SlopeBytesPerDay)),
		m.DaysUntilFull,
		m.ProjectedFullAt.Format("2006-01-02"),
	)
}

func nodesCapacityForecastUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes capacity-forecast <id> [flags]

Projects this node's disk and memory usage forward from their recent
trend (a simple linear fit, not a guarantee) and reports roughly how
many days remain until each is full. A resource with a flat or
improving trend is omitted, not shown as "fine forever".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the forecast as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

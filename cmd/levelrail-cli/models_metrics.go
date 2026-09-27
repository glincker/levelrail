package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runModelsMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models metrics", "print the engine metrics report as JSON to stdout and nothing else", stderr)
	var since time.Duration
	fs.DurationVar(&since, "since", time.Hour, "how far back to read, for example 6h")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models metrics <name> [flags]\n\nShows the engine's own metrics: KV cache usage, queue depth, prefix cache hits, tokens per second and time to first token. Metrics an engine cannot expose are marked not available.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models metrics", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	rep, err := client.GetModelEngineMetrics(context.Background(), name, since)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get engine metrics of model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printEngineMetrics(stdout, rep) })
}

func printEngineMetrics(out io.Writer, r apiclient.EngineMetricsReport) {
	_, _ = fmt.Fprintf(out, "health: %s (%s)\n\n", r.Health.State, r.Health.Summary)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "METRIC\tLATEST")
	for _, s := range r.Series {
		switch {
		case !s.Supported:
			_, _ = fmt.Fprintf(tw, "%s\tnot available for %s\n", s.Label, r.Engine)
		case s.Latest == nil:
			_, _ = fmt.Fprintf(tw, "%s\tno samples yet\n", s.Label)
		default:
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", s.Label, formatEngineMetric(*s.Latest, s.Unit))
		}
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\n%s\n", r.Note)
}

func formatEngineMetric(v float64, unit string) string {
	switch unit {
	case "percent":
		return fmt.Sprintf("%.1f%%", v)
	case "seconds":
		return fmt.Sprintf("%.0f ms", v*1000)
	case "tokens_per_second":
		return fmt.Sprintf("%.1f tok/s", v)
	case "bytes":
		return fmt.Sprintf("%.1f GiB", v/(1<<30))
	}
	return fmt.Sprintf("%.0f", v)
}

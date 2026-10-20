package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsOverview implements "apps overview [names...]": GET /api/v1/apps-metrics,
// the batched read the dashboard's apps list uses.
func runAppsOverview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps overview", "print the overview as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps overview [name ...] [flags]\n\nShows CPU, memory, request rate, 5xx error rate and p95 latency for every\napp you can read (or only the named apps) in a single request.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rows, err := client.ListAppMetricsSummary(context.Background(), fs.Args())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apps overview: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rows, func() { printAppsOverviewTable(stdout, rows) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAppsOverviewTable(out io.Writer, rows []appMetricsSummary) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no apps")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCPU\tMEMORY\tREQ/S\t5XX\tP95")
	for _, r := range rows {
		reqs, errs, p95 := "-", "-", "-"
		if r.HasTraffic {
			reqs = fmt.Sprintf("%.2f", r.RatePerSec)
			errs = fmt.Sprintf("%.1f%%", r.ErrorRate5xx*100)
			p95 = fmt.Sprintf("%.0fms", r.P95Ms)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Name, formatCPUPercent(r.CPUPercent), formatMemoryUsage(r.MemoryBytes, r.MemoryLimit), reqs, errs, p95)
	}
	_ = tw.Flush()
}

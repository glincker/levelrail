package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var vitalsCLIMetrics = []string{"LCP", "INP", "CLS", "TTFB", "FCP"}

// Shows real user web vitals (p75 per route) collected by the app's beacon.
func runAppsVitals(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps vitals", "print the web vitals report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps vitals <name> [flags]\n\nShows the 7 day p75 of LCP, INP, CLS, TTFB and FCP for one app, overall and\nper route, from the app's web vitals beacon.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps vitals", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	report, err := client.GetAppVitals(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get web vitals for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, report, func() { printAppVitalsHuman(stdout, report) })
}

func vitalsCell(metric string, m map[string]apiclient.AppVitalsStat) string {
	s, ok := m[metric]
	if !ok || s.Count == 0 {
		return "-"
	}
	v := strconv.FormatFloat(s.P75, 'f', -1, 64)
	if metric != "CLS" {
		v += "ms"
	}
	return v + " " + s.Rating
}

func printAppVitalsHuman(out io.Writer, r apiclient.AppVitalsResource) {
	if len(r.Overall) == 0 {
		_, _ = fmt.Fprintf(out, "No web vitals yet (collection enabled: %t).\n", r.Enabled)
		return
	}
	rows := [][]string{append([]string{"ROUTE"}, vitalsCLIMetrics...)}
	all := []string{"all routes"}
	for _, m := range vitalsCLIMetrics {
		all = append(all, vitalsCell(m, r.Overall))
	}
	rows = append(rows, all)
	for _, rt := range r.Routes {
		row := []string{rt.Route}
		for _, m := range vitalsCLIMetrics {
			row = append(row, vitalsCell(m, rt.Metrics))
		}
		rows = append(rows, row)
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, c := range row {
			widths[i] = max(widths[i], len(c))
		}
	}
	_, _ = fmt.Fprintf(out, "p75 over the last %d days\n", r.Days)
	for _, row := range rows {
		var b strings.Builder
		for i, c := range row {
			_, _ = fmt.Fprintf(&b, "%-*s  ", widths[i], c)
		}
		_, _ = fmt.Fprintln(out, strings.TrimRight(b.String(), " "))
	}
}

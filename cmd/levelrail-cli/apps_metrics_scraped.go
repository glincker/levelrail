package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// splitScrapedFlag routes `apps metrics --scraped` to the app's own scraped series.
func splitScrapedFlag(args []string) ([]string, bool) {
	rest := make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if a == "--scraped" || a == "-scraped" {
			found = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, found
}

func runAppsScrapedMetrics(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps metrics --scraped", "print the result as JSON to stdout and nothing else", stderr)
	var metric, since, from, to, path string
	var port int
	var off bool
	fs.StringVar(&metric, "metric", "", "scraped metric name to query (default: list stored names)")
	fs.StringVar(&since, "since", "", "how far back to query, e.g. \"1h\" (default: 1h)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the query window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the query window (default: now)")
	fs.IntVar(&port, "scrape-port", 0, "set the container port the app serves metrics on")
	fs.StringVar(&path, "scrape-path", "/metrics", "path used with --scrape-port")
	fs.BoolVar(&off, "scrape-off", false, "stop scraping this app")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsScrapedUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps metrics --scraped", "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	switch {
	case off:
		if err := client.DeleteAppScrapeTarget(ctx, name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("stop scraping app %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "Stopped scraping %s.\n", name)
		return exitOK
	case port != 0:
		res, err := client.SetAppScrapeTarget(ctx, name, port, path)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set scrape target for app %q: %w", name, err))
		}
		return renderScraped(stdout, stderr, of, res, func() {
			_, _ = fmt.Fprintf(stdout, "Scraping %s on container port %d at %s.\n", name, res.Port, res.Path)
		})
	}
	fromTime, toTime, err := resolveTimeRange(timeRangeFlags{since: since, from: from, to: to}, time.Now())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res, err := client.QueryAppScrapedMetrics(ctx, name, metric, fromTime, toTime)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query scraped metrics for app %q: %w", name, err))
	}
	return renderScraped(stdout, stderr, of, res, func() { printScrapedHuman(stdout, res) })
}

func renderScraped(stdout, stderr io.Writer, of outputFlags, v any, human func()) int {
	if err := renderResult(stdout, of.Format, of.Query, v, human); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printScrapedHuman(out io.Writer, res apiclient.AppScrapedMetricsResource) {
	if res.Metric == "" {
		if len(res.Names) == 0 {
			_, _ = fmt.Fprintln(out, "No scraped metrics stored yet.")
			return
		}
		for _, n := range res.Names {
			_, _ = fmt.Fprintln(out, n)
		}
		return
	}
	if len(res.Series) == 0 {
		_, _ = fmt.Fprintf(out, "No samples of %s in this window.\n", res.Metric)
		return
	}
	for _, s := range res.Series {
		last := s.Points[len(s.Points)-1]
		_, _ = fmt.Fprintf(out, "%s%s  %g  (%d points, last %s)\n", res.Metric, s.Labels, last.Value, len(s.Points), last.Timestamp.Format(time.RFC3339))
	}
}

func appsScrapedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps metrics <name> --scraped [--metric NAME] [--since 1h] [--from TIME] [--to TIME]
  %[1]s apps metrics <name> --scraped --scrape-port 9100 [--scrape-path /metrics]
  %[1]s apps metrics <name> --scraped --scrape-off

Reads series the control plane scraped from the app's own metrics endpoint.
The target is always the app container on its own network; only the port
and path are configurable.
`, prog)
}

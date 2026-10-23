package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const defaultTrafficInterval = 5 * time.Second

var trafficWindows = map[string]bool{"5m": true, "1h": true, "24h": true, "7d": true}

// runTraffic implements "traffic <app>": GET /api/v1/apps/{name}/traffic,
// request rate, errors and latency percentiles per app and per replica.
func runTraffic(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "export" {
		return runTrafficExport(prog, args[1:], os.Stdin, stdout, stderr, lookupEnv)
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "traffic", "print the traffic report as JSON to stdout and nothing else", stderr)
	var window, intervalStr string
	var watch bool
	fs.StringVar(&window, "window", "1h", "time window: 5m, 1h, 24h or 7d")
	fs.StringVar(&window, "since", "1h", "alias of --window")
	fs.BoolVar(&watch, "watch", false, "refresh the report until interrupted")
	fs.StringVar(&intervalStr, "interval", defaultTrafficInterval.String(), "how often --watch refreshes")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, trafficUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "traffic", "app name")
	if !ok {
		return exitUsage
	}
	if !trafficWindows[window] {
		return reportError(stdout, stderr, jsonOut, newValidationError("--window must be 5m, 1h, 24h or 7d, got %q", window))
	}
	interval, err := parseDurationOrZero(intervalStr)
	if err != nil || interval <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--interval must be a valid positive duration, got %q", intervalStr))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		res, err := client.GetAppTraffic(ctx, name, window)
		if err != nil {
			if ctx.Err() != nil {
				return exitOK
			}
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get traffic for app %q: %w", name, err))
		}
		if watch && jsonOut {
			err = writeJSONLine(stdout, res)
		} else {
			err = renderResult(stdout, of.Format, of.Query, res, func() { printTrafficHuman(stdout, res) })
		}
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		if !watch {
			return exitOK
		}
		select {
		case <-ctx.Done():
			return exitOK
		case <-time.After(interval):
		}
	}
}

func printTrafficHuman(out io.Writer, r apiclient.AppTrafficResource) {
	_, _ = fmt.Fprintf(out, "app: %s  window: %s\n", r.App, r.Window)
	if !r.HasTraffic {
		_, _ = fmt.Fprintln(out, "no requests in this window")
		return
	}
	_, _ = fmt.Fprintf(out, "requests: %.0f  rate: %.2f/s  2xx: %.0f  3xx: %.0f  4xx: %.0f (%.2f%%)  5xx: %.0f (%.2f%%)\n",
		r.Requests, r.RatePerSec, r.StatusClasses.Status2xx, r.StatusClasses.Status3xx,
		r.StatusClasses.Status4xx, r.ErrorRate4xx*100, r.StatusClasses.Status5xx, r.ErrorRate5xx*100)
	_, _ = fmt.Fprintf(out, "latency: p50 %.0fms  p95 %.0fms  p99 %.0fms\n", r.Latency.P50Ms, r.Latency.P95Ms, r.Latency.P99Ms)
	if r.Shield.Blocked > 0 {
		parts := make([]string, 0, len(r.Shield.ByReason))
		for _, b := range r.Shield.ByReason {
			if b.Count > 0 {
				parts = append(parts, fmt.Sprintf("%s %.0f", b.Reason, b.Count))
			}
		}
		_, _ = fmt.Fprintf(out, "shield blocked: %.0f (%s)\n", r.Shield.Blocked, strings.Join(parts, ", "))
	}
	if r.SLO != nil {
		_, _ = fmt.Fprintf(out, "slo: %s %.2f%%  burn %.2fx  budget left %.1f%%\n", r.SLO.Objective, r.SLO.Target, r.SLO.BurnRate, r.SLO.BudgetRemaining*100)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	if len(r.Replicas) > 0 {
		_, _ = fmt.Fprintln(out, "\nreplicas:")
		_, _ = fmt.Fprintln(tw, "REPLICA\tREQ\tREQ/S\t5XX%\tP50MS\tP95MS\tP99MS")
		for _, rep := range r.Replicas {
			_, _ = fmt.Fprintf(tw, "%s\t%.0f\t%.2f\t%.2f\t%.0f\t%.0f\t%.0f\n", rep.Upstream, rep.Requests, rep.RatePerSec, rep.ErrorRate5xx*100, rep.P50Ms, rep.P95Ms, rep.P99Ms)
		}
		_ = tw.Flush()
	}
	if len(r.TopRoutes) > 0 {
		_, _ = fmt.Fprintln(out, "\ntop paths:")
		_, _ = fmt.Fprintln(tw, "ROUTE\tREQ\tAVGMS\t4XX%\t5XX%")
		for _, rt := range r.TopRoutes {
			_, _ = fmt.Fprintf(tw, "%s\t%.0f\t%.0f\t%.2f\t%.2f\n", rt.Route, rt.Requests, rt.AvgMs, rt.ErrorRate4xx*100, rt.ErrorRate5xx*100)
		}
		_ = tw.Flush()
	}
	if len(r.SlowRoutes) > 0 {
		_, _ = fmt.Fprintln(out, "\nslowest routes:")
		_, _ = fmt.Fprintln(tw, "ROUTE\tREQ\tAVGMS\t5XX%")
		for _, rt := range r.SlowRoutes {
			_, _ = fmt.Fprintf(tw, "%s\t%.0f\t%.0f\t%.2f\n", rt.Route, rt.Requests, rt.AvgMs, rt.ErrorRate5xx*100)
		}
		_ = tw.Flush()
	}
	printTrafficSpikes(out, tw, r.ErrorSpikes)
	printTrafficLabels(out, tw, "top referrers:", "REFERRER", r.Referrers)
	printTrafficLabels(out, tw, "top countries:", "COUNTRY", r.Countries)
	if len(r.Samples) > 0 {
		_, _ = fmt.Fprintln(out, "\nslow and failed requests:")
		_, _ = fmt.Fprintln(tw, "TIME\tSTATUS\tMETHOD\tROUTE\tMS\tREPLICA\tREQUEST ID")
		for _, s := range r.Samples {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%.0f\t%s\t%s\n", s.Time.Format(time.RFC3339), s.Status, s.Method, s.Route, s.DurationMS, s.Upstream, s.RequestID)
		}
		_ = tw.Flush()
	}
}

func printTrafficSpikes(out io.Writer, tw *tabwriter.Writer, spikes []apiclient.TrafficErrorSpike) {
	if len(spikes) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\n5xx spikes (newest first):")
	_, _ = fmt.Fprintln(tw, "START\tEND\tPEAK 5XX%\tBASELINE%\t5XX\tDEPLOY")
	for _, s := range spikes {
		deploy := "-"
		if s.DeployID != "" {
			deploy = fmt.Sprintf("%s %s", s.DeployID, s.DeployImage)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%.1f\t%.1f\t%.0f\t%s\n", s.Start.Format(time.RFC3339), s.End.Format(time.RFC3339), s.PeakRate5xx*100, s.BaselineRate*100, s.Errors5xx, deploy)
	}
	_ = tw.Flush()
}

func printTrafficLabels(out io.Writer, tw *tabwriter.Writer, title, header string, rows []apiclient.TrafficLabel) {
	if len(rows) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\n"+title)
	_, _ = fmt.Fprintln(tw, header+"\tREQ\tSHARE")
	for _, l := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%.0f\t%.1f%%\n", l.Label, l.Requests, l.Share*100)
	}
	_ = tw.Flush()
}

func trafficUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s traffic <app> [--window|--since 1h] [--watch] [--json] [flags]

Shows an app's request rate, errors and latency percentiles (p50, p95, p99)
as measured at the ingress, with a per replica breakdown, the slowest route
templates, 5xx spikes tied to the deploy that preceded them, top referrer hosts and countries (countries need a geo source),
sampled slow or failed requests and the SLO error-budget burn.

Flags:
  --window string         time window: 5m, 1h, 24h or 7d (default 1h)
  --watch                   refresh the report until interrupted
  --interval string       how often --watch refreshes (default %[5]s)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the report as JSON to stdout, one line per refresh with --watch
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, defaultTrafficInterval)
}

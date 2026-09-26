package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsRequests implements "apps requests <name>" (also reachable as
// "apps metrics <name> --requests"): GET /api/v1/apps/{name}/requests, the
// ingress request rate, 4xx/5xx error rates and latency percentiles.
func runAppsRequests(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps requests", "print the request series as JSON to stdout and nothing else", stderr)
	var since, from, to, step string
	fs.StringVar(&since, "since", "", "how far back to query, e.g. \"1h\", \"30m\" (default: 1h; mutually exclusive with --from)")
	fs.StringVar(&from, "from", "", "RFC3339 start of the query window (overrides --since)")
	fs.StringVar(&to, "to", "", "RFC3339 end of the query window (default: now)")
	fs.StringVar(&step, "step", "", "bucket size, e.g. \"1m\" (default: chosen by the server)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRequestsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps requests", "app name")
	if !ok {
		return exitUsage
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
	res, err := client.QueryAppRequests(context.Background(), name, fromTime, toTime, stepDuration)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("query requests for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printRequestsHuman(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRequestsHuman(out io.Writer, res apiclient.AppRequestsResource) {
	s := res.Summary
	if !s.HasTraffic {
		_, _ = fmt.Fprintf(out, "app: %s\nno requests in range\n", res.App)
		return
	}
	_, _ = fmt.Fprintf(out, "app: %s\nrequests: %.0f  rate: %.2f/s  4xx: %.2f%%  5xx: %.2f%%  p95: %.0fms\n",
		res.App, s.Requests, s.RatePerSec, s.ErrorRate4xx*100, s.ErrorRate5xx*100, s.P95Ms)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TIMESTAMP\tREQ/S\t4XX%\t5XX%\tP50MS\tP95MS\tP99MS")
	for _, p := range res.Points {
		_, _ = fmt.Fprintf(tw, "%s\t%.2f\t%.2f\t%.2f\t%.0f\t%.0f\t%.0f\n",
			p.Timestamp.Format(time.RFC3339), p.RatePerSec, p.ErrorRate4xx*100, p.ErrorRate5xx*100, p.P50Ms, p.P95Ms, p.P99Ms)
	}
	_ = tw.Flush()
}

func appsRequestsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps requests <name> [flags]
  %[1]s apps metrics <name> --requests [flags]

Shows an app's request rate, 4xx/5xx error rates and latency percentiles as
measured at the ingress, with no app changes.

Flags:
  --since string          how far back to query, e.g. "1h", "30m" (default: 1h)
  --from string            RFC3339 start of the query window (overrides --since)
  --to string                RFC3339 end of the query window (default: now)
  --step string            bucket size, e.g. "1m" (default: chosen by the server)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the request series as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// splitRequestsFlag removes a --requests/-requests flag from args and reports
// whether it was present.
func splitRequestsFlag(args []string) ([]string, bool) {
	rest := make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if a == "--requests" || a == "-requests" {
			found = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, found
}

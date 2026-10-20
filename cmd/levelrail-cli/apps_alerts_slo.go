package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const sloKind = "slo_burn"

// sloFlags are the --slo* flags "apps alerts create" and "update" share.
type sloFlags struct {
	target    float64
	latencyMs float64
}

func registerSLOFlags(fs *flag.FlagSet) *sloFlags {
	s := &sloFlags{}
	fs.Float64Var(&s.target, "slo", 0, "SLO target as a percentage of good requests, e.g. 99.9 (--kind slo_burn only, required for that kind)")
	fs.Float64Var(&s.latencyMs, "slo-latency-ms", 0, "make it a latency SLO: a request is good when it finishes within this many ms (--kind slo_burn only, optional)")
	return s
}

func (s *sloFlags) config() apiclient.AlertSLOConfig {
	cfg := apiclient.AlertSLOConfig{Objective: "availability", Target: s.target, LatencyMs: s.latencyMs}
	if s.latencyMs > 0 {
		cfg.Objective = "latency"
	}
	return cfg
}

func (s *sloFlags) apply(kind string, req *createAlertRuleRequest) error {
	if kind != sloKind {
		if s.target != 0 || s.latencyMs != 0 {
			return newValidationError("--slo and --slo-latency-ms apply to --kind slo_burn only")
		}
		return nil
	}
	if s.target <= 0 {
		return newValidationError("--slo is required for --kind slo_burn (a target such as 99.9)")
	}
	cfg := s.config()
	req.SLO = &cfg
	return nil
}

func describeSLO(c *apiclient.AlertSLOConfig) string {
	if c == nil {
		return "-"
	}
	if c.Objective == "latency" {
		return fmt.Sprintf("%g%% of requests under %gms, 30d budget", c.Target, c.LatencyMs)
	}
	return fmt.Sprintf("%g%% availability, 30d budget", c.Target)
}

func runAppsAlertsSLO(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps alerts slo", "print the preview as JSON to stdout and nothing else", stderr)
	slo := &sloFlags{target: 99.9}
	fs.Float64Var(&slo.target, "slo", 99.9, "SLO target as a percentage of good requests")
	fs.Float64Var(&slo.latencyMs, "slo-latency-ms", 0, "preview a latency SLO with this threshold in ms")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsAlertsSLOUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, "apps alerts slo", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	preview, err := client.GetSLOPreview(context.Background(), appName, slo.config())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview slo for app %q: %w", appName, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, preview, func() { printSLOPreview(stdout, preview) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printSLOPreview(out io.Writer, p *apiclient.SLOPreviewResource) {
	_, _ = fmt.Fprintf(out, "slo:              %s\n", describeSLO(&p.Config))
	if !p.HasTraffic {
		_, _ = fmt.Fprintln(out, "no request traffic recorded in the budget window, nothing to burn yet")
		return
	}
	_, _ = fmt.Fprintf(out, "budget remaining: %.1f%% (over %.0f requests)\n", p.BudgetRemaining*100, p.BudgetRequests)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\nTIER\tBURN (long/short)\tTHRESHOLD\tSTATE")
	for _, t := range p.Tiers {
		kind := "ticket"
		if t.Page {
			kind = "page"
		}
		state := "ok"
		if t.Firing {
			state = "FIRING (" + kind + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%.2fx / %.2fx\t%gx over %s and %s\t%s\n", strings.ReplaceAll(t.Name, "_", " "),
			t.LongBurn, t.ShortBurn, t.Factor, secs(t.LongSeconds), secs(t.ShortSeconds), state)
	}
	_ = tw.Flush()
}

func secs(s int64) string {
	d := time.Duration(s) * time.Second
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

func appsAlertsSLOUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps alerts slo <app> [--slo 99.9] [--slo-latency-ms N] [flags]

Shows the error budget left and the current burn rate for each alert tier
of a request-based SLO, computed from the app's ingress request metrics the
same way an slo_burn rule evaluates them. Nothing is created. Create the
rule with:
  %[1]s apps alerts create <app> --name NAME --kind slo_burn --slo 99.9

Burn rate is how many times faster than sustainable the error budget is
being spent: 1x uses exactly the budget over the 30 day window. Defaults
(override with APP_SLO_* env vars on the control plane): 14.4x over 1h and
5m and 6x over 6h and 30m page, 3x over 1d and 2h and 1x over 3d and 6h
open a ticket.

Flags:
  --slo float             target percentage of good requests (default 99.9)
  --slo-latency-ms float  preview a latency SLO with this threshold in ms
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read
  --json                  print the preview as JSON to stdout, nothing else
  --output string         output format: json, table, or text
  --query string          JMESPath expression to filter the result before printing
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

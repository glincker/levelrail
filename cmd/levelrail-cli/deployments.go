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

// runDeployments dispatches "deployments list|watch|summary".
func runDeployments(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, deploymentsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, deploymentsUsage(prog))
		return exitOK
	case "list":
		return runDeploymentsList(prog, rest, stdout, stderr, lookupEnv)
	case "watch":
		return runDeploymentsWatch(prog, rest, stdout, stderr, lookupEnv)
	case "summary":
		return runDeploymentsSummary(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown deployments subcommand %q\n\n%s", prog, args[0], deploymentsUsage(prog))
	return exitUsage
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func runDeploymentsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "deployments list", "print the deployments response as JSON to stdout and nothing else", stderr)
	var (
		status, trigger string
		opts            apiclient.DeploymentListOptions
	)
	fs.StringVar(&status, "status", "", "comma-separated statuses: building, ready, failed, canceled, rolled_back, superseded, held")
	fs.StringVar(&trigger, "trigger", "", "comma-separated triggers: git push, manual, rollback, api, preview")
	fs.StringVar(&opts.App, "app", "", "only this app")
	fs.StringVar(&opts.Branch, "branch", "", "only this branch")
	fs.StringVar(&opts.Environment, "environment", "", "only this environment (name or id)")
	fs.StringVar(&opts.Since, "since", "", "only deployments started after this RFC3339 time or duration ago, e.g. 24h or 7d")
	fs.StringVar(&opts.Until, "until", "", "only deployments started before this RFC3339 time or duration ago")
	fs.StringVar(&opts.Query, "q", "", "search commit message, commit sha prefix and app name")
	fs.BoolVar(&opts.Live, "live", false, "only the release currently serving each app")
	fs.IntVar(&opts.PR, "pr", 0, "only previews of this pull request number")
	fs.IntVar(&opts.Limit, "limit", 0, "page size (default 50, max 200)")
	fs.StringVar(&opts.Cursor, "cursor", "", "continue from a next_cursor of a previous page")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deploymentsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: deployments list takes no arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	opts.Statuses, opts.Triggers = splitCSV(status), splitCSV(trigger)

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListDeployments(context.Background(), opts)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deployments: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printDeploymentsTable(stdout, list) })
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return dashIfEmpty(s)
}

func formatDurationMS(ms *int64) string {
	if ms == nil {
		return "-"
	}
	return (time.Duration(*ms) * time.Millisecond).Round(100 * time.Millisecond).String()
}

func printDeploymentsTable(out io.Writer, list apiclient.DeploymentList) {
	if len(list.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no deployments")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tAPP\tSTATUS\tTRIGGER\tENV\tCOMMIT\tBRANCH\tDURATION\tSTARTED")
	for _, d := range list.Items {
		status := d.Status
		if d.IsLive {
			status += " (live)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", d.ID, d.App, status, d.Trigger,
			dashIfEmpty(d.Environment), shortSHA(d.CommitSHA), dashIfEmpty(d.Branch), formatDurationMS(d.DurationMS), d.StartedAt.Format(time.RFC3339))
	}
	_ = tw.Flush()
	if list.NextCursor != "" {
		_, _ = fmt.Fprintf(out, "more: rerun with --cursor %s\n", list.NextCursor)
	}
}

func runDeploymentsSummary(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "deployments summary", "print the summary as JSON to stdout and nothing else", stderr)
	var window string
	fs.StringVar(&window, "window", "", "counting window such as 24h or 7d, at most 30d (default 24h)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deploymentsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: deployments summary takes no arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	s, err := client.GetDeploymentsSummary(context.Background(), window)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deployments summary: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, s, func() { printDeploymentsSummary(stdout, s) })
}

func printDeploymentsSummary(out io.Writer, s apiclient.DeploymentSummary) {
	_, _ = fmt.Fprintf(out, "window: %s\n", s.Window)
	for _, k := range []string{"building", "ready", "failed", "canceled", "rolled_back", "superseded", "held"} {
		_, _ = fmt.Fprintf(out, "%-12s %d\n", k, s.Counts[k])
	}
	_, _ = fmt.Fprintf(out, "in progress: %d, needs attention: %d\n", s.InProgress, s.NeedsAttention)
	if s.FailureRate24h != nil {
		_, _ = fmt.Fprintf(out, "failure rate 24h: %.1f%%\n", *s.FailureRate24h*100)
	} else {
		_, _ = fmt.Fprintln(out, "failure rate 24h: -")
	}
	_, _ = fmt.Fprintf(out, "duration median: %s, p95: %s\n", formatDurationMS(s.Duration.MedianMS), formatDurationMS(s.Duration.P95MS))
}

func runDeploymentsWatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "deployments watch", "print one JSON object per event instead of text", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, deploymentsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "%s: deployments watch takes no arguments\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := client.StreamDeployments(ctx, func(ev apiclient.DeploymentEvent) error {
		if jsonOut {
			return writeJSONLine(stdout, ev)
		}
		d := ev.Deployment
		detail := ""
		if ev.Step != nil {
			detail = " " + ev.Step.Name + " " + ev.Step.Status
		}
		_, err := fmt.Fprintf(stdout, "%s  %-8s %s %s (%s)%s\n", time.Now().Format(time.RFC3339), ev.Type, d.App, d.ID, d.Status, detail)
		return err
	})
	if ctx.Err() != nil {
		return exitOK
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("watch deployments: %w", err))
	}
	return exitOK
}

func deploymentsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deployments list [filters] [flags]      deployments across every app you can read, newest first
  %[1]s deployments summary [--window 24h]      counts by status, failure rate, duration percentiles
  %[1]s deployments watch [flags]               stream deployment changes until interrupted

List filters:
  --status a,b      building, ready, failed, canceled, rolled_back, superseded, held
  --trigger a,b     git push, manual, rollback, api, preview
  --app NAME        --branch NAME      --environment NAME
  --since T         --until T          RFC3339 time or duration ago (24h, 7d)
  --q TEXT          commit message, sha prefix or app name
  --live            only the release currently serving each app
  --pr N            only previews of pull request N
  --limit N         page size (default 50, max 200)
  --cursor C        continue from a previous page's next_cursor

Common flags:
  --token, --api-url, --profile, --json, --output, --query, -h
`, prog)
}

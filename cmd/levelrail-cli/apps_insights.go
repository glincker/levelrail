package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsSummary implements "apps summary": fleet status counts.
func runAppsSummary(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps summary", "print the summary as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps summary [flags]\n\nShows how many apps are running, failing, deploying, stopped or unknown.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	sum, err := client.GetAppsSummary(context.Background(), apiclient.AppListQuery{})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get apps summary: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, sum, func() {
		_, _ = fmt.Fprintf(stdout, "total %d: running %d, failing %d, deploying %d, stopped %d, unknown %d\n",
			sum.Total, sum.Running, sum.Failing, sum.Deploying, sum.Stopped, sum.Unknown)
	})
}

// runAppsChanges implements "apps changes <name>": what changed recently, likely cause flagged.
func runAppsChanges(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps changes", "print the changes as JSON to stdout and nothing else", stderr)
	var window string
	fs.StringVar(&window, "window", "", "look-back window as a duration from 1s to 24h (default: server setting)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps changes <name> [--window 1h] [flags]\n\nLists deploys, config, env, secret and scaling changes in the recent window,\nmarking the likely cause of a regression.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps changes", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.GetAppChanges(context.Background(), name, time.Time{}, window)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get changes for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAppChangesHuman(stdout, res) })
}

func printAppChangesHuman(out io.Writer, res *apiclient.RecentChangesResource) {
	if len(res.Changes) == 0 {
		_, _ = fmt.Fprintln(out, "no changes in the window")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WHEN\tKIND\tACTOR\tWHAT\tLIKELY CAUSE")
	for _, c := range res.Changes {
		what := c.Title
		if len(c.Keys) > 0 {
			what += " (" + strings.Join(c.Keys, ", ") + ")"
		}
		cause := ""
		if c.LikelyCause {
			cause = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.At.Local().Format("2006-01-02 15:04:05"), c.Kind, c.Actor, what, cause)
	}
	_ = tw.Flush()
	if res.Truncated {
		_, _ = fmt.Fprintf(out, "showing %d of %d changes\n", len(res.Changes), res.Total)
	}
}

// runAppsDeploysProbes implements "apps deploys probes <name> <deploy-id>".
func runAppsDeploysProbes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys probes", "print the probe attempts as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps deploys probes <name> <deploy-id> [flags]\n\nLists each readiness-probe attempt a deploy's cutover made, with status\ncode and latency.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys probes", "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	attempts, err := client.ListProbeAttempts(context.Background(), rest[0], rest[1])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list probe attempts for app %q deploy %q: %w", rest[0], rest[1], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, attempts, func() { printProbeAttemptsHuman(stdout, attempts) })
}

func printProbeAttemptsHuman(out io.Writer, attempts []apiclient.ProbeAttempt) {
	if len(attempts) == 0 {
		_, _ = fmt.Fprintln(out, "no probe attempts recorded")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WHEN\tTARGET\tRESULT\tCODE\tLATENCY\tERROR")
	for _, a := range attempts {
		result := "fail"
		if a.Success {
			result = "ok"
		}
		code := "-"
		if a.StatusCode != 0 {
			code = fmt.Sprint(a.StatusCode)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%dms\t%s\n", formatTimelineTime(a.ProbedAt), a.Target, result, code, a.LatencyMS, a.Error)
	}
	_ = tw.Flush()
}

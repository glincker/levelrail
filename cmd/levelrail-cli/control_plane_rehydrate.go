package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const rehydratePollEnv = "APP_REHYDRATE_POLL_INTERVAL"

// runControlPlane dispatches "control-plane rehydrate plan|run|status".
func runControlPlane(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, controlPlaneUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "rehydrate":
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, controlPlaneUsage(prog))
		return exitOK
	default:
		_, _ = fmt.Fprint(stderr, controlPlaneUsage(prog))
		return exitUsage
	}
	if len(args) < 2 {
		_, _ = fmt.Fprint(stderr, controlPlaneUsage(prog))
		return exitUsage
	}
	switch args[1] {
	case "plan":
		return runRehydratePlan(prog, args[2:], stdout, stderr, lookupEnv)
	case "run":
		return runRehydrateRun(prog, args[2:], stdout, stderr, lookupEnv)
	case "status":
		return runRehydrateStatus(prog, args[2:], stdout, stderr, lookupEnv)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, controlPlaneUsage(prog))
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown control-plane rehydrate subcommand %q\n\n", prog, args[1])
		_, _ = fmt.Fprint(stderr, controlPlaneUsage(prog))
		return exitUsage
	}
}

func controlPlaneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s control-plane rehydrate plan [flags]     dry run: which databases and volumes would be restored, and from which backup
  %[1]s control-plane rehydrate run --yes        restore them, with a per-resource result and a DR report
  %[1]s control-plane rehydrate status [flags]   the live or last DR report

Run these after the control plane itself is restored. A run overwrites live
databases and volumes, so it needs --yes. Backups older than
APP_REHYDRATE_MAX_BACKUP_AGE block their resource unless --allow-stale is set.
All subcommands need a root-scoped token.
`, prog)
}

func rehydrateFlags(prog, label string, stderr io.Writer) (*flag.FlagSet, apiFlagPtrs) {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane rehydrate "+label, "print the report as JSON to stdout and nothing else", stderr)
	return fs, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}
}

func runRehydratePlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := rehydrateFlags(prog, "plan", stderr)
	allowStale := fs.Bool("allow-stale", false, "plan restores from backups older than the age limit instead of blocking them")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, ptrs, stderr, lookupEnv)
	if !ok {
		return code
	}
	rep, err := client.PlanRehydrate(context.Background(), *allowStale)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plan rehydrate: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printRehydrateReport(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if rep.Summary.Blocked > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func runRehydrateRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := rehydrateFlags(prog, "run", stderr)
	yes := fs.Bool("yes", false, "confirm that live databases and volumes may be overwritten")
	allowStale := fs.Bool("allow-stale", false, "restore from backups older than the age limit instead of blocking them")
	noWait := fs.Bool("no-wait", false, "return as soon as the run has started")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, ptrs, stderr, lookupEnv)
	if !ok {
		return code
	}
	if !*yes {
		return reportError(stdout, stderr, jsonOut, newValidationError("run overwrites live databases and volumes: review %s control-plane rehydrate plan, then pass --yes", prog))
	}
	ctx := context.Background()
	rep, err := client.RunRehydrate(ctx, *allowStale)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start rehydrate: %w", err))
	}
	if !*noWait {
		rep, err = waitRehydrate(ctx, client, lookupEnv)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printRehydrateReport(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if rep.State == "failed" || rep.State == "partial" {
		return exitCheckFailed
	}
	return exitOK
}

func runRehydrateStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, ptrs := rehydrateFlags(prog, "status", stderr)
	client, jsonOut, of, code, ok := drClient(prog, fs, args, ptrs, stderr, lookupEnv)
	if !ok {
		return code
	}
	rep, err := client.GetRehydrateReport(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get rehydrate report: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printRehydrateReport(stdout, rep) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func waitRehydrate(ctx context.Context, client *Client, lookupEnv func(string) (string, bool)) (apiclient.RehydrateReport, error) {
	interval := 2 * time.Second
	if v, ok := lookupEnv(rehydratePollEnv); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			interval = d
		}
	}
	for {
		select {
		case <-ctx.Done():
			return apiclient.RehydrateReport{}, ctx.Err()
		case <-time.After(interval):
		}
		rep, err := client.GetRehydrateReport(ctx)
		if err != nil {
			return apiclient.RehydrateReport{}, fmt.Errorf("get rehydrate report: %w", err)
		}
		if rep.State != "running" {
			return rep, nil
		}
	}
}

func printRehydrateReport(out io.Writer, rep apiclient.RehydrateReport) {
	_, _ = fmt.Fprintf(out, "%s report %s: %s\n", rep.Mode, rep.ID, rep.State)
	if len(rep.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no databases or app volumes to restore")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RESOURCE\tSTATUS\tBACKUP AGE\tDETAIL")
	for _, it := range rep.Items {
		age, detail := "-", it.Reason
		if it.Backup != nil {
			age = (time.Duration(it.Backup.AgeSeconds) * time.Second).String()
		}
		if it.Error != "" {
			detail = it.Error
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.ID, it.Status, age, detail)
	}
	_ = tw.Flush()
	s := rep.Summary
	_, _ = fmt.Fprintf(out, "restorable %d, succeeded %d, failed %d, blocked %d, skipped %d, oldest backup used %s\n",
		s.Restorable, s.Succeeded, s.Failed, s.Blocked, s.Skipped, (time.Duration(s.WorstBackupAgeSeconds) * time.Second).String())
}

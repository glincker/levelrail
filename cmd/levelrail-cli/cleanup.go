package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	cleanupModeOff     = "off"
	cleanupModePreview = "preview"
	cleanupModeOn      = "on"
	cleanupFormatCSV   = "csv"
	cleanupDaysUnset   = -1
)

// runCleanup dispatches list/preview/run/mode/runs over the Cleanup center.
func runCleanup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cleanupUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cleanupUsage(prog))
		return exitOK
	case "list":
		return runCleanupList(prog, args[1:], stdout, stderr, lookupEnv)
	case "preview":
		return runCleanupPreview(prog, args[1:], stdout, stderr, lookupEnv)
	case "run":
		return runCleanupRun(prog, args[1:], stdout, stderr, lookupEnv)
	case "mode":
		return runCleanupMode(prog, args[1:], stdout, stderr, lookupEnv)
	case "runs":
		return runCleanupRuns(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown cleanup subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, cleanupUsage(prog))
		return exitUsage
	}
}

func cleanupUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cleanup list [flags]                          every cleanup target with usage, retention, mode and last run
  %[1]s cleanup preview ID [--days N] [flags]         rows and bytes a run would delete (deletes nothing)
  %[1]s cleanup run ID [--days N] [--dry-run] [--yes] [flags]
                                                      delete old data; without --dry-run, --yes is required
  %[1]s cleanup mode ID off|preview|on [flags]        off, preview (scheduled runs only record) or on
  %[1]s cleanup runs [--target ID] [--since DATE] [--format table|json|csv] [flags]
                                                      run history, newest first

Retention is a runtime setting (see "%[1]s settings runtime"). --days overrides it for one call.
Viewing needs a read token; preview, run and mode need a root API token.

Run "%[1]s cleanup <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runCleanupList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cleanup list", "print the cleanup targets as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cleanup list [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if _, ok := requireArgs(fs, stderr, prog, "cleanup list", "no arguments", 0); !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.GetCleanup(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list cleanup targets: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printCleanupTargetsTable(stdout, list) })
}

func runCleanupMode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cleanup mode", "print the updated target as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cleanup mode ID off|preview|on [flags]\n\noff never runs the target. preview makes scheduled runs record what would be\ndeleted without deleting. on lets the scheduler delete.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "cleanup mode", "a target ID and off, preview or on", 2)
	if !ok {
		return exitUsage
	}
	id, mode := rest[0], rest[1]
	if !isCleanupMode(mode) {
		_, _ = fmt.Fprintf(stderr, "%s: cleanup mode must be off, preview or on, got %q\n", prog, mode)
		return exitValidation
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetCleanupMode(context.Background(), id, mode)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set cleanup mode for %s: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "%s mode: %s\n", id, mode)
	})
}

func runCleanupRuns(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cleanup runs", "print the run history as JSON to stdout and nothing else", stderr)
	target := fs.String("target", "", "only runs of this cleanup target ID")
	since := fs.String("since", "", "only runs started on or after this date (YYYY-MM-DD or RFC 3339)")
	format := fs.String("format", "", "output format: table, json or csv (overrides --output)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cleanup runs [--target ID] [--since DATE] [--format table|json|csv] [flags]\n\nLists scheduled, manual and CLI cleanup runs, dry runs included.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if _, ok := requireArgs(fs, stderr, prog, "cleanup runs", "no arguments", 0); !ok {
		return exitUsage
	}
	sinceValue, err := parseCleanupSince(*since)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	switch *format {
	case "":
	case string(outputTable):
		of.Format = outputTable
	case string(outputJSON):
		of.Format = outputJSON
	case cleanupFormatCSV:
	default:
		_, _ = fmt.Fprintf(stderr, "%s: --format must be table, json or csv, got %q\n", prog, *format)
		return exitValidation
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	page, err := client.ListCleanupRuns(context.Background(), apiclient.CleanupRunsQuery{Target: *target, Since: sinceValue})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list cleanup runs: %w", err))
	}
	if *format == cleanupFormatCSV {
		return writeCleanupRunsCSV(stdout, stderr, page.Runs)
	}
	return writeScheduledTaskResult(stdout, stderr, of, page, func() { printCleanupRunsTable(stdout, page.Runs) })
}

// parseCleanupSince accepts a calendar date or RFC 3339 and returns RFC 3339 in UTC, or "" when unset.
func parseCleanupSince(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if t, err := time.Parse(time.DateOnly, raw); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("--since must be YYYY-MM-DD or RFC 3339, got %q", raw)
}

// cleanupDaysArg returns nil when --days was not passed, so the target's retention policy applies.
func cleanupDaysArg(fs *flag.FlagSet, days int, stderr io.Writer, prog string) (*int, bool) {
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "days" {
			given = true
		}
	})
	if !given {
		return nil, true
	}
	if days < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: --days must be at least 1, got %d\n", prog, days)
		return nil, false
	}
	return &days, true
}

func isCleanupMode(mode string) bool {
	return mode == cleanupModeOff || mode == cleanupModePreview || mode == cleanupModeOn
}

func writeCleanupRunsCSV(stdout, stderr io.Writer, runs []apiclient.CleanupRunResource) int {
	w := csv.NewWriter(stdout)
	_ = w.Write([]string{"id", "target", "started_at", "finished_at", "days", "dry_run", "rows", "bytes", "trigger", "actor", "error"})
	for _, r := range runs {
		_ = w.Write([]string{
			strconv.FormatInt(r.ID, 10), r.Target, r.StartedAt, r.FinishedAt, strconv.Itoa(r.Days),
			strconv.FormatBool(r.DryRun), strconv.FormatInt(r.Rows, 10), strconv.FormatInt(r.Bytes, 10),
			r.Trigger, r.Actor, r.Error,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printCleanupTargetsTable(out io.Writer, list apiclient.CleanupListResource) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCATEGORY\tDAYS\tMODE\tROWS\tLAST RUN")
	for _, t := range list.Targets {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", t.ID, t.Category, cleanupDaysLabel(t.Days), t.Mode, t.Usage.Rows, cleanupLastRunLabel(t.LastRun))
	}
	_ = tw.Flush()
	state := "disabled"
	if list.Scheduler.Enabled {
		state = "enabled"
	}
	_, _ = fmt.Fprintf(out, "\nscheduler %s, interval %s\n", state, list.Scheduler.Interval)
}

func printCleanupRunsTable(out io.Writer, runs []apiclient.CleanupRunResource) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTARGET\tSTARTED\tDAYS\tDRY RUN\tROWS\tBYTES\tTRIGGER\tERROR")
	for _, r := range runs {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", r.ID, r.Target, r.StartedAt, cleanupDaysLabel(r.Days), runtimeYesNo(r.DryRun), r.Rows, r.Bytes, r.Trigger, r.Error)
	}
	_ = tw.Flush()
}

func cleanupDaysLabel(days int) string {
	if days == 0 {
		return "forever"
	}
	return strconv.Itoa(days)
}

func cleanupLastRunLabel(r *apiclient.CleanupRunResource) string {
	if r == nil {
		return "never"
	}
	label := r.StartedAt
	if r.DryRun {
		label += " (dry run)"
	}
	if r.Error != "" {
		label += " (failed: " + strings.TrimSpace(r.Error) + ")"
	}
	return label
}

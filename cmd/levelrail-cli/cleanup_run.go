package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runCleanupPreview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cleanup preview", "print the impact as JSON to stdout and nothing else", stderr)
	days := fs.Int("days", cleanupDaysUnset, "retention window to preview instead of the target's policy")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cleanup preview ID [--days N] [flags]\n\nReports how many rows and bytes a run would delete. Deletes nothing.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "cleanup preview", "exactly one ID", 1)
	if !ok {
		return exitUsage
	}
	daysArg, ok := cleanupDaysArg(fs, *days, stderr, prog)
	if !ok {
		return exitValidation
	}

	id := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	impact, err := client.PreviewCleanup(context.Background(), id, daysArg)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview cleanup %s: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, impact, func() {
		_, _ = fmt.Fprintf(stdout, "%s would delete %d rows (%d bytes)\n", id, impact.Rows, impact.Bytes)
	})
}

func runCleanupRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cleanup run", "print the run result as JSON to stdout and nothing else", stderr)
	days := fs.Int("days", cleanupDaysUnset, "retention window to apply for this run instead of the target's policy")
	dryRun := fs.Bool("dry-run", false, "record what would be deleted without deleting anything")
	yes := fs.Bool("yes", false, "confirm the deletion that the preview reports (required without --dry-run)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cleanup run ID [--days N] [--dry-run] [--yes] [flags]\n\nDeletes old data for one cleanup target. Without --dry-run the preview row count is\nprinted first and --yes is required. A dry run is audited like any other run.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "cleanup run", "exactly one ID", 1)
	if !ok {
		return exitUsage
	}
	daysArg, ok := cleanupDaysArg(fs, *days, stderr, prog)
	if !ok {
		return exitValidation
	}

	id := rest[0]
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if !*dryRun {
		impact, err := client.PreviewCleanup(ctx, id, daysArg)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview cleanup %s: %w", id, err))
		}
		_, _ = fmt.Fprintf(stderr, "%s would delete %d rows (%d bytes)\n", id, impact.Rows, impact.Bytes)
		if !*yes {
			return reportError(stdout, stderr, jsonOut, newValidationError("refusing to delete %d rows from %s without --yes (add --dry-run to only record the preview)", impact.Rows, id))
		}
	}

	result, err := client.RunCleanup(ctx, id, daysArg, *dryRun)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("run cleanup %s: %w", id, err))
	}
	code := writeScheduledTaskResult(stdout, stderr, of, result, func() { printCleanupRunResult(stdout, id, result) })
	if code == exitOK && result.Error != "" {
		_, _ = fmt.Fprintf(stderr, "%s: cleanup %s failed: %s\n", prog, id, result.Error)
		return exitAPIError
	}
	return code
}

func printCleanupRunResult(out io.Writer, id string, r apiclient.CleanupRunResult) {
	verb := "deleted"
	if r.DryRun {
		verb = "dry run: would delete"
	}
	_, _ = fmt.Fprintf(out, "%s %s %d rows (%d bytes)\n", id, verb, r.Rows, r.Bytes)
	if r.MoreRemaining {
		_, _ = fmt.Fprintln(out, "more rows remain past this run's cap, run again to continue")
	}
}

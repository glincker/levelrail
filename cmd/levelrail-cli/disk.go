package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func diskUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s disk gc [--dry-run | --apply] [--if-needed]   unused images and idle build cache on the control plane node

gc previews by default and removes only with --apply. Images used by an app,
rollback targets and the newest deploys are kept. --if-needed makes --apply
do nothing while free space is above the cleanup budget.
`, prog)
}

func runDisk(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, diskUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, diskUsage(prog))
		return exitOK
	case "gc":
		return runDiskGC(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown disk subcommand %q\n\n%s", prog, args[0], diskUsage(prog))
	return exitUsage
}

func runDiskGC(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "disk gc", "print the report as JSON to stdout and nothing else", stderr)
	dry := fs.Bool("dry-run", false, "preview only (the default)")
	apply := fs.Bool("apply", false, "remove what the report lists")
	ifNeeded := fs.Bool("if-needed", false, "with --apply, act only when free space is below the budget")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, diskUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *dry && *apply {
		_, _ = fmt.Fprintln(stderr, prog+": disk gc: --dry-run and --apply cannot be combined")
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.RunDiskGC(context.Background(), apiclient.DiskGCRequest{Apply: *apply, IfNeeded: *ifNeeded})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disk gc: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printDiskGC(stdout, rep) })
}

func printDiskGC(out io.Writer, rep apiclient.DiskGCReport) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CATEGORY\tITEMS\tRECLAIMABLE\tRECLAIMED")
	for _, c := range rep.Categories {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", c.Category, c.Count, humanBytes(c.ReclaimBytes), humanBytes(c.ReclaimedBytes))
		if c.Error != "" {
			_, _ = fmt.Fprintf(tw, "%s\tfailed\t\t%s\n", c.Category, c.Error)
		}
	}
	_ = tw.Flush()
	switch {
	case rep.Skipped != "":
		_, _ = fmt.Fprintf(out, "nothing removed: %s (%.1f%% free, budget %.0f%%)\n", rep.Skipped, rep.FreePercent, rep.BudgetPercent)
	case rep.DryRun:
		_, _ = fmt.Fprintf(out, "preview only, %s reclaimable\n", humanBytes(rep.TotalReclaimBytes))
	default:
		_, _ = fmt.Fprintf(out, "reclaimed %s\n", humanBytes(rep.TotalReclaimed))
	}
}

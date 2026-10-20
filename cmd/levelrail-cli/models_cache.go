package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runModelsCache dispatches "models cache list|prune".
func runModelsCache(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s models cache list [flags]                  list cached model weights per node\n  %[1]s models cache prune [--dry-run] [volume...]  remove unused, unreferenced weights\n", prog)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "list":
		return runModelsCacheList(prog, args[1:], stdout, stderr, lookupEnv)
	case "prune":
		return runModelsCachePrune(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown models cache subcommand %q\n", prog, args[0])
	return exitUsage
}

func runModelsCacheList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models cache list", "print the cache report as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models cache list [flags]\n\nLists cached model weights per node with size and last use.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	report, err := client.ListModelCache(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list model cache: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, report, func() { printModelCache(stdout, report) })
}

func runModelsCachePrune(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models cache prune", "print the prune result as JSON to stdout and nothing else", stderr)
	var dryRun bool
	fs.BoolVar(&dryRun, "dry-run", false, "list what would be removed without removing anything")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models cache prune [--dry-run] [volume...] [flags]\n\nRemoves cached weights that no model uses and that are unused past\nAPP_MODEL_CACHE_UNUSED_DAYS. Volumes mounted by a container or owned by a\nconfigured model are never removed. Run with --dry-run first.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.PruneModelCache(context.Background(), fs.Args(), dryRun)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("prune model cache: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printCachePrune(stdout, res) })
}

func printModelCache(out io.Writer, r apiclient.ModelCacheReport) {
	for _, n := range r.Nodes {
		_, _ = fmt.Fprintf(out, "%s: %s cached (%s unique), %s reclaimable, disk free %s\n", n.Name, humanBytes(n.TotalBytes), humanBytes(n.UniqueBytes), humanBytes(n.ReclaimableBytes), optBytes(n.DiskFreeBytes))
		if n.Message != "" {
			_, _ = fmt.Fprintf(out, "  %s\n", n.Message)
		}
		if len(n.Entries) == 0 {
			continue
		}
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  VOLUME\tMODEL\tSIZE\tUNUSED\tSTATE")
		for _, e := range n.Entries {
			state := "prunable"
			if !e.Prunable {
				state = "kept: " + e.KeepReason
			}
			if e.DuplicateOf != "" {
				state += " (same weights as " + e.DuplicateOf + ")"
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%dd\t%s\n", e.Volume, e.Model, optBytes(e.SizeBytes), e.UnusedDays, state)
		}
		_ = tw.Flush()
	}
	_, _ = fmt.Fprintf(out, "\nUnused means no use for %d days. %s\n", r.UnusedDays, r.Note)
}

func printCachePrune(out io.Writer, r apiclient.ModelCachePruneResult) {
	verb := "removed"
	names := r.Removed
	if r.DryRun {
		verb = "would remove"
		names = nil
		for _, c := range r.Candidates {
			names = append(names, c.Volume)
		}
	}
	if len(names) == 0 {
		_, _ = fmt.Fprintln(out, "nothing to prune")
	}
	for _, n := range names {
		_, _ = fmt.Fprintf(out, "%s %s\n", verb, n)
	}
	for _, s := range r.Skipped {
		_, _ = fmt.Fprintf(out, "kept %s: %s\n", s.Volume, s.Reason)
	}
	if !r.DryRun {
		_, _ = fmt.Fprintf(out, "reclaimed %s\n", humanBytes(r.ReclaimedBytes))
	}
}

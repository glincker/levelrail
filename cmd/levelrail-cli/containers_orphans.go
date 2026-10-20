package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runContainersOrphans implements "containers orphans": GET
// /api/v1/system/orphans, the same definition the reaper acts on.
func runContainersOrphans(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runOrphanReport(prog, "containers orphans", args, stdout, stderr, lookupEnv, false, false)
}

// runContainersReap implements "containers reap [--dry-run]": POST
// /api/v1/system/orphans/reap, one reaper pass on demand.
func runContainersReap(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runOrphanReport(prog, "containers reap", args, stdout, stderr, lookupEnv, true, true)
}

func runOrphanReport(prog, name string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), reap, allowDry bool) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, name, "print the report as JSON to stdout and nothing else", stderr)
	var dry bool
	if allowDry {
		fs.BoolVar(&dry, "dry-run", false, "report what would be removed without removing anything")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s [flags]\n\nFlags:\n", prog, name)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var (
		rep orphanReport
		err error
	)
	if reap {
		rep, err = client.ReapOrphans(context.Background(), dry)
	} else {
		rep, err = client.ListOrphans(context.Background())
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rep, func() { printOrphanReport(stdout, rep, reap) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if len(rep.Failed) > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func printOrphanReport(out io.Writer, rep orphanReport, reaped bool) {
	if len(rep.Findings) == 0 {
		_, _ = fmt.Fprintln(out, "no orphans")
	} else {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tNAME\tNODE\tREASON\tSTATUS")
		for _, f := range rep.Findings {
			node := f.NodeID
			if node == "" {
				node = "local"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", f.Kind, f.Name, node, f.Reason, orphanStatus(f))
		}
		_ = tw.Flush()
	}
	if reaped {
		verb := "removed"
		if rep.DryRun {
			verb = "would be removed"
		}
		dueCount := 0
		for _, f := range rep.Findings {
			if f.Due {
				dueCount++
			}
		}
		if rep.DryRun {
			_, _ = fmt.Fprintf(out, "%d %s\n", dueCount, verb)
		} else {
			_, _ = fmt.Fprintf(out, "%d %s\n", len(rep.Removed), verb)
		}
	}
	for _, f := range rep.Failed {
		_, _ = fmt.Fprintf(out, "failed: %s %s: %s\n", f.Kind, f.Name, f.Error)
	}
	for _, n := range rep.Unreachable {
		_, _ = fmt.Fprintf(out, "node %s was not scanned (unreachable)\n", n)
	}
	if rep.Halted != "" {
		_, _ = fmt.Fprintf(out, "note: %s\n", rep.Halted)
	}
}

func orphanStatus(f orphanFinding) string {
	switch {
	case f.Skip != "":
		return "kept: " + f.Skip
	case f.Due:
		return "due"
	case f.ReapAfter != nil:
		return "grace until " + *f.ReapAfter
	}
	return "pending"
}

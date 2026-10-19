package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runSystemIntegrity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "system integrity", "print the database integrity state as JSON to stdout and nothing else", stderr)
	check := fs.Bool("check", false, "run a check now (quick unless --full)")
	full := fs.Bool("full", false, "with --check, run the full integrity_check instead of the quick one")
	restore := fs.String("restore", "", "stage a verified snapshot to replace the database at the next start: a snapshot name, or \"latest\"")
	cancel := fs.Bool("cancel-restore", false, "cancel a staged restore")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, systemUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	switch {
	case *restore != "":
		name := *restore
		if name == "latest" {
			name = ""
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.IntegrityRestore, error) {
			return client.RestoreIntegrity(ctx, name, p)
		})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("stage restore: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() { printIntegrityRestore(stdout, res) })
	case *cancel:
		if err := client.CancelIntegrityRestore(ctx); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("cancel restore: %w", err))
		}
	case *check:
		if _, err := client.CheckIntegrity(ctx, *full); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("integrity check: %w", err))
		}
	}
	view, err := client.GetIntegrity(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("database integrity: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, view, func() { printIntegrity(stdout, view) })
}

func printIntegrityRestore(out io.Writer, r apiclient.IntegrityRestore) {
	_, _ = fmt.Fprintf(out, "staged %s (schema %d). It replaces the database at the next start.\n", r.Pending.Name, r.Pending.SchemaVersion)
	if r.Restart.Supported {
		_, _ = fmt.Fprintln(out, "apply it now with: system restart")
		return
	}
	_, _ = fmt.Fprintf(out, "restart the service on the host to apply it: %s\n", r.Restart.Hint)
}

func printIntegrity(out io.Writer, v apiclient.Integrity) {
	state := "ok"
	if !v.Status.Healthy {
		state = "DAMAGED: " + v.Status.Error
	}
	_, _ = fmt.Fprintf(out, "database:  %s\nlast check: %s (%s), %d checks, %d failures\n", state, v.Status.CheckedAt.Format("2006-01-02 15:04:05Z07:00"), v.Status.Mode, v.Status.Checks, v.Status.Failures)
	if v.Pending != nil {
		_, _ = fmt.Fprintf(out, "staged restore: %s (applies at the next start; cancel with --cancel-restore)\n", v.Pending.Name)
	}
	if len(v.Candidates) == 0 {
		_, _ = fmt.Fprintln(out, "snapshots: none")
		return
	}
	_, _ = fmt.Fprintln(out, "snapshots (newest first):")
	for _, c := range v.Candidates {
		mark := "not verified"
		if c.Verified {
			mark = "verified"
		}
		_, _ = fmt.Fprintf(out, "  %-40s %8d KiB  %s\n", c.Name, c.SizeBytes>>10, mark)
	}
}

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func printAttemptStatus(out io.Writer, s apiclient.SelfUpgradeStatus) {
	if r := s.Running; r != nil {
		_, _ = fmt.Fprintf(out, "upgrade running: %s to %s (attempt %s, step %s, started %s)\n", r.FromVersion, r.ToVersion, r.ID, r.CurrentStep, r.StartedAt)
	}
	if l := s.LastFinished; l != nil {
		verified := ""
		if l.Outcome == "succeeded" {
			verified = ", verification failed"
			if s.Verified {
				verified = ", verification passed"
			}
		}
		_, _ = fmt.Fprintf(out, "last upgrade:    %s, %s to %s, finished %s (attempt %s%s)\n", l.Outcome, l.FromVersion, l.ToVersion, l.FinishedAt, l.ID, verified)
	}
}

// runUpgradeStatus implements "upgrade status": the install's upgrade state
// and the reason, read-only (no backup is taken).
func runUpgradeStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "upgrade status", "print the status as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	pre, err := client.GetUpdatePreflight(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade status: %w", err))
	}
	attempts, attemptsErr := client.GetSelfUpgradeStatus(context.Background())
	if err := renderResult(stdout, of.Format, of.Query, pre, func() {
		printUpgradeStatus(stdout, pre)
		if attemptsErr == nil {
			printAttemptStatus(stdout, attempts)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printUpgradeStatus(out io.Writer, p updatePreflight) {
	_, _ = fmt.Fprintf(out, "running version: %s\n", p.CurrentVersion)
	if p.LatestVersion != nil {
		_, _ = fmt.Fprintf(out, "latest release:  %s\n", *p.LatestVersion)
	}
	_, _ = fmt.Fprintf(out, "state:           %s\n", p.State)
	_, _ = fmt.Fprintf(out, "reason:          %s\n", p.StateReason)
	if p.DBSchema != nil {
		_, _ = fmt.Fprintf(out, "database schema: %d\n", *p.DBSchema)
	}
	if p.State == "upgrade_available" && p.MigrationsToApply != nil && *p.MigrationsToApply >= 0 {
		_, _ = fmt.Fprintf(out, "migrations:      %d (a restore point is taken first, then a %ds verify window)\n", *p.MigrationsToApply, p.VerifyWindowSeconds)
	}
	if p.State == "ahead_of_releases" && p.RollbackVersion != "" {
		_, _ = fmt.Fprintf(out, "rollback:        retained release %s (see `upgrade --rollback-plan %s`)\n", p.RollbackVersion, p.RollbackVersion)
	}
}

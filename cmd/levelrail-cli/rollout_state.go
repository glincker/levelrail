package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runRolloutState handles "rollout state <app>": the gate, traffic flip and
// hold countdown of the newest release.
func runRolloutState(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "rollout state"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the rollout state as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, rolloutUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetRolloutState(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printRolloutState(stdout, res) })
}

func printRolloutState(w io.Writer, r apiclient.RolloutStateResource) {
	_, _ = fmt.Fprintf(w, "app %s: strategy %s, stage %s, traffic %s\n", r.App, r.Strategy, r.Stage, r.Traffic)
	if r.Image != "" {
		_, _ = fmt.Fprintf(w, "  release %s (%s)\n", r.DeployID, r.Image)
	}
	if r.GateFailures > 0 {
		_, _ = fmt.Fprintf(w, "  health gate failed %d time(s): %s\n", r.GateFailures, r.FailureReason)
	}
	if r.FlippedAt != nil {
		_, _ = fmt.Fprintf(w, "  traffic flipped at %s\n", r.FlippedAt.Format("2006-01-02 15:04:05"))
	}
	h := r.Hold
	_, _ = fmt.Fprintf(w, "  hold %s (mode %s, auto-rollback %t): %ds left of %ds\n", h.State, h.Mode, r.AutoRollback, h.RemainingSeconds, h.WindowSeconds)
	if r.RolledBackAt != nil {
		_, _ = fmt.Fprintf(w, "  rolled back at %s: %s\n", r.RolledBackAt.Format("2006-01-02 15:04:05"), r.FailureReason)
	}
	if r.Canary != nil && r.Canary.Active {
		_, _ = fmt.Fprintf(w, "  canary %s at %d%%\n", r.Canary.Image, r.Canary.Weight)
	}
}

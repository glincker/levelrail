package main

import (
	"context"
	"fmt"
	"io"
)

// Separate from apps get: its --json shape is a stable script contract that must not gain conditions.
func runAppsStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps status", "print conditions as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps status <name> [flags]\n\nShows the application controller's current stored reconcile conditions\nfor one app (current status, not a deploy history log).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps status", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	conditions, err := client.GetDeployStatus(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get status for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, conditions, func() {
		printConditionsHuman(stdout, conditions)
		printStatusExtras(context.Background(), client, prog, name, stdout, conditions)
	})
}

// printStatusExtras adds the lines conditions alone cannot show: changes
// saved but not yet running, and how long the previous release is kept.
// Best effort: a failure just omits the line.
func printStatusExtras(ctx context.Context, client *Client, prog, name string, out io.Writer, conditions []conditionResource) {
	if pending, err := client.GetPendingChanges(ctx, name); err == nil && pending.Pending {
		// A restart in flight recreates the container from saved state, so
		// "apps apply" would only restart it a second time.
		for _, c := range conditions {
			if c.Type == "Ready" && c.Reason == "RollingOut" {
				_, _ = fmt.Fprintf(out, "\npending changes: %s (being applied by the restart in progress)\n", describePendingChanges(pending))
				return
			}
		}
		_, _ = fmt.Fprintf(out, "\npending changes: %s (run: %s apps apply %s)\n", describePendingChanges(pending), prog, name)
	}
	if app, err := client.GetApp(ctx, name); err == nil && app.PreviousReleaseHeldUntil != "" {
		_, _ = fmt.Fprintf(out, "previous release held until %s (instant rollback target)\n", formatTimelineTime(app.PreviousReleaseHeldUntil))
	}
}

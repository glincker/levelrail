package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsStatus implements "apps status <name>": GET
// /api/v1/apps/{name}/deploys (internal/api/deploys.go's own
// handleDeployHistory), the application controller's current stored
// reconcile conditions, the same data the web frontend's ConditionsPanel
// reads. Deliberately a separate command from "apps get" rather than
// folded into it: the two endpoints return genuinely different resource
// shapes (desired app state vs. observed reconcile status), and "apps
// get --json" already has a stable appResource contract a script might
// depend on, so changing its shape to also carry conditions would be a
// breaking change to that contract for no reason a new command doesn't
// solve just as well.
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
		printStatusExtras(context.Background(), client, prog, name, stdout)
	})
}

// printStatusExtras adds the lines conditions alone cannot show: changes
// saved but not yet running, and how long the previous release is kept.
// Best effort: a failure just omits the line.
func printStatusExtras(ctx context.Context, client *Client, prog, name string, out io.Writer) {
	if pending, err := client.GetPendingChanges(ctx, name); err == nil && pending.Pending {
		_, _ = fmt.Fprintf(out, "\npending changes: %s (run: %s apps apply %s)\n", describePendingChanges(pending), prog, name)
	}
	if app, err := client.GetApp(ctx, name); err == nil && app.PreviousReleaseHeldUntil != "" {
		_, _ = fmt.Fprintf(out, "previous release held until %s (instant rollback target)\n", formatTimelineTime(app.PreviousReleaseHeldUntil))
	}
}

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

// failureCodeContainerCrashed is the deploy failure code for a container that exited.
const failureCodeContainerCrashed = "container_crashed"

// crashLoopLine explains a crash loop from the stored Ready condition, or
// returns "" when the app is not in one. forCrash also yields a generic line
// for a crashed release that has not hit the restart backoff yet.
func crashLoopLine(conditions []conditionResource, forCrash bool) string {
	for _, c := range conditions {
		if c.Reason != deploysafety.ReasonCrashLoopBackOff {
			continue
		}
		if n, wait, ok := deploysafety.ParseCrashLoop(c.Message); ok {
			return fmt.Sprintf("crash loop detected: the release restarted %d times, next retry in %s (backoff, no retry limit)", n, wait.Round(time.Second))
		}
		return "crash loop detected: the release keeps exiting and restarts are backing off"
	}
	if forCrash {
		return "container crash: the release exited; the control plane retries with a growing delay, see apps status for the next retry"
	}
	return ""
}

// fetchAppStatus prefers the status route and falls back to the bare
// conditions list when it fails or reports nothing, so older servers keep working.
func fetchAppStatus(ctx context.Context, client *Client, name string) (apiclient.AppStatus, error) {
	st, err := client.GetAppStatus(ctx, name)
	if err == nil && (len(st.Conditions) > 0 || st.Outcome != "") {
		return st, nil
	}
	conds, cerr := client.GetDeployStatus(ctx, name)
	if cerr != nil {
		return st, err
	}
	st.Conditions = conds
	return st, nil
}

// statusOutcomeLine prints the server's reason when it sent an outcome and
// only then falls back to the local crash-loop wording.
func statusOutcomeLine(st apiclient.AppStatus, conditions []conditionResource) string {
	if st.Outcome == "" {
		return crashLoopLine(conditions, false)
	}
	if st.Outcome == apiclient.DeployOutcomeHealthy || st.OutcomeReason == "" {
		return ""
	}
	return fmt.Sprintf("latest deploy %s: %s", st.Outcome, st.OutcomeReason)
}

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

	status, err := fetchAppStatus(context.Background(), client, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get status for app %q: %w", name, err))
	}
	conditions := status.Conditions

	return writeScheduledTaskResult(stdout, stderr, of, conditions, func() {
		printConditionsHuman(stdout, conditions)
		if line := statusOutcomeLine(status, conditions); line != "" {
			_, _ = fmt.Fprintf(stdout, "\n%s\n", line)
		}
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

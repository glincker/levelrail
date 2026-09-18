package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// rolloutOutcome is what "apps wait" actually needs to know about a
// deploy attempt's roll-out: has it converged yet, and if so, did it
// succeed. Ports web/src/lib/deployStages.ts's own computeRolloutStage
// to Go, since the CLI has no way to share that TypeScript and needs
// the identical convergence rule to give a CI caller the same answer
// the dashboard would show.
type rolloutOutcome struct {
	// state is "pending" (still converging), "succeeded", or "failed".
	state     string
	condition *conditionResource // the matching condition, nil if state is "pending"
}

// rolloutFailureReasons and rolloutDoneReasons mirror deployStages.ts's
// own ROLLOUT_FAILURE_REASONS/ROLLOUT_DONE_REASONS exactly: every reason
// string internal/reconcile/application/controller.go's
// ensureReplicaRunning can actually report. Keep these two lists and
// that file's own reason strings in sync; see deployStages.ts's own
// comment on the bug that let this list and the reconciler's actual
// reasons drift apart once already.
var rolloutFailureReasons = map[string]bool{
	"CreateFailed":             true,
	"StartFailed":              true,
	"ReadinessFailed":          true,
	"InspectFailed":            true,
	"EnsureNetworkFailed":      true,
	"VanishedAfterStart":       true,
	"PreDeployHookFailed":      true,
	"OOMKilledDuringReadiness": true,
	"ExitedDuringReadiness":    true,
}

var rolloutDoneReasons = map[string]bool{
	"Deployed":       true,
	"AlreadyRunning": true,
}

// computeRolloutOutcome decides attempt's rollout outcome from
// conditions, the app's current reconcile conditions. Pure and
// side-effect-free, matching computeRolloutStage's own shape:
//   - attempt.Status == "failed" (the build itself failed, an image-
//     source attempt never has this): rollout never started, reported
//     as failed here too since apps wait has no separate "skipped"
//     concept a CI caller would act on differently.
//   - attempt.Status == "running": still building, pending.
//   - attempt.FinishedAt == nil: shouldn't happen once status is
//     terminal, treated as pending defensively.
//   - otherwise: a failure-reason condition transitioning at or after
//     FinishedAt wins over a done-reason one (checked first, matching
//     computeRolloutStage), else a done-reason condition at or after
//     FinishedAt means success, else still pending.
func computeRolloutOutcome(attempt deployAttemptResource, conditions []conditionResource) rolloutOutcome {
	if attempt.Status == "failed" {
		return rolloutOutcome{state: "failed"}
	}
	if attempt.Status == "running" || attempt.FinishedAt == nil {
		return rolloutOutcome{state: "pending"}
	}
	reference := *attempt.FinishedAt

	for i := range conditions {
		c := &conditions[i]
		if rolloutFailureReasons[c.Reason] && !c.LastTransitionTime.Before(reference) {
			return rolloutOutcome{state: "failed", condition: c}
		}
	}
	for i := range conditions {
		c := &conditions[i]
		if rolloutDoneReasons[c.Reason] && !c.LastTransitionTime.Before(reference) {
			return rolloutOutcome{state: "succeeded", condition: c}
		}
	}
	return rolloutOutcome{state: "pending"}
}

// deployAttemptFetcher is waitForRollout's dependency, satisfied by
// *Client in production and a fake in tests: fetching both resources
// every poll (not just conditions) is what lets computeRolloutOutcome
// tell "converged" apart from "the reconciler hasn't caught up to this
// exact attempt yet," the same reasoning useDeployProgress.ts's own doc
// comment gives for polling both queries.
type deployAttemptFetcher interface {
	ListDeployAttempts(ctx context.Context, name string) ([]deployAttemptResource, error)
	GetDeployStatus(ctx context.Context, name string) ([]conditionResource, error)
}

// waitForRollout polls client every pollInterval until name's target
// attempt (attemptID, or the latest attempt if empty) converges, ctx is
// done, or an API call fails. onTick, if non-nil, is called once per
// poll with the current outcome (including "pending" ones), so a caller
// can print live progress without waitForRollout itself owning any
// output.
func waitForRollout(ctx context.Context, client deployAttemptFetcher, name, attemptID string, pollInterval time.Duration, onTick func(rolloutOutcome)) (rolloutOutcome, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		attempts, err := client.ListDeployAttempts(ctx, name)
		if err != nil {
			return rolloutOutcome{}, fmt.Errorf("list deploy attempts for app %q: %w", name, err)
		}
		attempt, found := findDeployAttempt(attempts, attemptID)
		if !found {
			if attemptID != "" {
				return rolloutOutcome{}, fmt.Errorf("deploy attempt %q not found for app %q", attemptID, name)
			}
			return rolloutOutcome{}, fmt.Errorf("app %q has no deploy attempts yet", name)
		}

		conditions, err := client.GetDeployStatus(ctx, name)
		if err != nil {
			return rolloutOutcome{}, fmt.Errorf("get deploy status for app %q: %w", name, err)
		}

		outcome := computeRolloutOutcome(attempt, conditions)
		if onTick != nil {
			onTick(outcome)
		}
		if outcome.state != "pending" {
			return outcome, nil
		}

		select {
		case <-ctx.Done():
			return rolloutOutcome{state: "pending"}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// findDeployAttempt returns attemptID's row (attempts is newest-first,
// so an empty attemptID means attempts[0], the latest) or ok=false if
// attempts is empty or attemptID names one that isn't in the list.
func findDeployAttempt(attempts []deployAttemptResource, attemptID string) (deployAttemptResource, bool) {
	if attemptID == "" {
		if len(attempts) == 0 {
			return deployAttemptResource{}, false
		}
		return attempts[0], true
	}
	for _, a := range attempts {
		if a.ID == attemptID {
			return a, true
		}
	}
	return deployAttemptResource{}, false
}

// runAppsWait implements "apps wait <name>": polls until the app's
// latest deploy attempt (or a specific one via --attempt-id) has
// actually converged, succeeded or failed, and exits accordingly.
// "apps deploy" itself returns the instant its desired-state write
// lands (202 Accepted), well before the reconciler has run even once
// (see apps_deploy.go's own doc comment); a CI job that wants to know
// whether a deploy genuinely succeeded, not just that the trigger call
// was accepted, needs this command as its own step afterward.
func runAppsWait(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps wait", "print the final outcome as JSON to stdout and nothing else", stderr)
	var attemptID, timeoutStr, pollIntervalStr string
	fs.StringVar(&attemptID, "attempt-id", "", "wait on this specific deploy attempt instead of the latest one")
	fs.StringVar(&timeoutStr, "timeout", "5m", "give up and exit non-zero after this long")
	fs.StringVar(&pollIntervalStr, "poll-interval", "3s", "how often to re-check")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsWaitUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps wait", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	timeout, err := parseDurationOrZero(timeoutStr)
	if err != nil || timeout <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must be a valid positive duration, got %q", timeoutStr))
	}
	pollInterval, err := parseDurationOrZero(pollIntervalStr)
	if err != nil || pollInterval <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--poll-interval must be a valid positive duration, got %q", pollIntervalStr))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	outcome, err := waitForRollout(ctx, client, name, attemptID, pollInterval, func(o rolloutOutcome) {
		if !jsonOut {
			_, _ = fmt.Fprintf(stderr, "waiting for %q to converge... (%s)\n", name, o.state)
		}
	})

	type waitResult struct {
		App    string             `json:"app"`
		Status string             `json:"status"`
		Reason string             `json:"reason,omitempty"`
		Detail *conditionResource `json:"condition,omitempty"`
	}

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			result := waitResult{App: name, Status: "timeout"}
			_ = renderResult(stdout, of.Format, of.Query, result, func() {
				_, _ = fmt.Fprintf(stdout, "timed out after %s waiting for %q to converge\n", timeout, name)
			})
			return exitDeployTimeout
		}
		return reportError(stdout, stderr, jsonOut, err)
	}

	result := waitResult{App: name, Status: outcome.state, Detail: outcome.condition}
	if outcome.condition != nil {
		result.Reason = outcome.condition.Reason
	}
	_ = renderResult(stdout, of.Format, of.Query, result, func() {
		if outcome.state == "succeeded" {
			_, _ = fmt.Fprintf(stdout, "%q converged successfully (%s)\n", name, result.Reason)
		} else {
			_, _ = fmt.Fprintf(stdout, "%q failed to converge (%s)\n", name, result.Reason)
		}
	})
	if outcome.state == "succeeded" {
		return exitOK
	}
	return exitDeployFailed
}

func appsWaitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps wait <name> [flags]

Polls until name's latest deploy attempt (or a specific one via
--attempt-id) has actually converged, then exits: 0 on success, %[5]d if
the rollout converged as a failure, %[6]d on timeout. "apps deploy"
itself returns as soon as its desired-state write lands, well before
the reconciler has run even once; this command is the way to actually
know whether that deploy succeeded, e.g. as a step in a CI pipeline
right after "apps deploy".

Flags:
  --attempt-id string      wait on this specific deploy attempt id instead of the latest one
  --timeout string           give up and exit %[6]d after this long (Go duration, default 5m)
  --poll-interval string   how often to re-check (Go duration, default 3s)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the final outcome as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, exitDeployFailed, exitDeployTimeout)
}

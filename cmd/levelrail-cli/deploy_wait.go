package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// deployWaitPollInterval is the fixed interval waitForDeployToConverge
// polls at. Not configurable in v1: --wait-timeout is the only knob a
// caller needs (how long to wait), not how often to ask.
const deployWaitPollInterval = 2 * time.Second

// defaultDeployWaitTimeout is --wait-timeout's default: long enough for
// a real image pull and health check, short enough that a genuinely
// stuck deploy doesn't hang a CI job indefinitely.
const defaultDeployWaitTimeout = 10 * time.Minute

// errDeployWaitTimeout is returned by waitForDeployToConverge when
// timeout elapses before the app's "Ready" condition reflects a
// reconcile pass triggered by this deploy.
var errDeployWaitTimeout = errors.New("timed out waiting for the deploy to converge")

// deployWaitOutcome is what waitForDeployToConverge settles on once a
// fresh "Ready" condition arrives: whether the reconciler actually
// converged, plus its own reason/message for reporting either way.
type deployWaitOutcome struct {
	Succeeded bool
	Reason    string
	Message   string
}

// deployReadySince returns name's currently stored "Ready" condition
// LastTransitionTime, or the zero time if none is stored yet. Call this
// once, before triggering a deploy or rollback, to snapshot a baseline:
// waitForDeployToConverge only trusts a "Ready" condition dated after
// this baseline, since the trigger call itself only ever updates desired
// state (apps_deploy.go's own doc comment on runAppsDeploy); it is the
// application controller's own next reconcile pass, not the trigger,
// that writes a fresh condition. Comparing against this previous
// server-side timestamp rather than the CLI process's local wall clock
// sidesteps any clock skew between this machine and the control plane.
//
// A failure here does not block the deploy itself, only --wait's own
// ability to tell a fresh convergence apart from an already-passing
// stale one: logged to stderr and treated as a zero baseline, the same
// "a status read must never block the write it's layered on" tradeoff
// internal/api/deploys.go's recordInstantDeployAttempt already makes.
func deployReadySince(ctx context.Context, client *Client, name string, stderr io.Writer) time.Time {
	conditions, err := client.GetDeployStatus(ctx, name)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: could not read %q's current status before waiting, --wait timing may be less precise: %v\n", name, err)
		return time.Time{}
	}
	for _, c := range conditions {
		if c.Type == "Ready" {
			return c.LastTransitionTime
		}
	}
	return time.Time{}
}

// waitForDeployToConverge polls GET .../deploys (client.GetDeployStatus,
// the same data "apps status" already shows) every
// deployWaitPollInterval until the application controller reports a
// "Ready" condition dated after since, or timeout elapses. Returns a
// non-nil error only when status itself could not be checked (a real
// API/network failure) or timeout was reached; a deploy that converges
// to a failed state is reported through the returned outcome, not an
// error.
//
// sleep is injected so tests can drive every iteration without a real
// wait, the same shape pollDeviceAuthUntilGranted (auth_login_device.go)
// already uses for its own poll loop; unlike that loop, sleep here also
// takes ctx so a cancelled context is honored immediately rather than
// riding out the rest of the current interval (see realDeploySleep).
func waitForDeployToConverge(ctx context.Context, client *Client, name string, since time.Time, timeout time.Duration, sleep func(context.Context, time.Duration), progress io.Writer) (deployWaitOutcome, error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return deployWaitOutcome{}, err
		}

		conditions, err := client.GetDeployStatus(ctx, name)
		if err != nil {
			return deployWaitOutcome{}, fmt.Errorf("check deploy status for %q: %w", name, err)
		}

		for _, c := range conditions {
			if c.Type != "Ready" || !c.LastTransitionTime.After(since) {
				continue
			}
			switch c.Status {
			case "True":
				return deployWaitOutcome{Succeeded: true, Reason: c.Reason, Message: c.Message}, nil
			case "False":
				return deployWaitOutcome{Succeeded: false, Reason: c.Reason, Message: c.Message}, nil
			}
			// Unknown: the controller has reconciled since our baseline
			// but hasn't reached a terminal verdict yet (e.g. the app was
			// suspended mid-deploy); keep polling.
		}

		if !time.Now().Before(deadline) {
			return deployWaitOutcome{}, errDeployWaitTimeout
		}
		if progress != nil {
			_, _ = fmt.Fprint(progress, ".")
		}
		sleep(ctx, deployWaitPollInterval)
	}
}

// realDeploySleep waits d or until ctx is cancelled, whichever comes
// first: the production sleep waitForDeployToConverge's real callers
// pass, so a cancelled command exits immediately rather than riding out
// the rest of the current poll interval.
func realDeploySleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// reportDeployWaitFailure prints err (and, in --json mode, writes it as
// stdout's {"error": ...} body, matching reportError's own shape) and
// always exits exitAPIError: a --wait outcome of "failed" or "timed out"
// means the deploy itself did not succeed, the same exit-code bucket
// this CLI already uses for a reached-but-unsuccessful API call, as
// opposed to exitUsage/exitValidation (a mistake in the command itself)
// or exitNetwork (the control plane was never reached at all, which by
// definition did happen here since status was polled successfully).
func reportDeployWaitFailure(stdout, stderr io.Writer, jsonOut bool, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	if jsonOut {
		if jerr := writeJSONError(stdout, err); jerr != nil {
			_, _ = fmt.Fprintln(stderr, jerr)
		}
	}
	return exitAPIError
}

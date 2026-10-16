package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// errStepsDone stops the step stream once the pipeline has ended; the server
// holds the connection open after the last step.
var errStepsDone = errors.New("deploy steps finished")

// deployStepsTerminal reports whether ev ends the build phase: any failure,
// or the final deploying step completing.
func deployStepsTerminal(ev deployStepEvent) bool {
	return ev.Status == "failed" || (ev.Step == "deploying" && ev.Status == "done")
}

// stepSummary is the closing step every traced deploy ends with.
const stepSummary = "deploy_summary"

// legacyBuildSteps are the coarse steps a build deploy emits before the
// reconciler's own trace starts.
var legacyBuildSteps = map[string]bool{"detecting": true, "building": true, "pushing": true, "deploying": true, "failed": true}

// deployStepsFinal reports whether ev ends the whole trace, and whether it
// ended in failure. A failed reconcile step is not final on its own: the
// reconciler retries, and only the summary step carries the verdict.
func deployStepsFinal(ev deployStepEvent) (final, failed bool) {
	switch {
	case ev.Step == stepSummary && ev.Status != "running":
		return true, ev.Status == "failed"
	case legacyBuildSteps[ev.Step] && ev.Status == "failed":
		return true, true
	}
	return false, false
}

// stepsGrace is how long to wait for the reconciler's summary after a build's
// last step; a static site never produces one.
const stepsGrace = 2 * time.Minute

func formatStepLine(ev deployStepEvent) string {
	name := ev.Step
	if ev.Key != "" && ev.Key != ev.Step {
		name = ev.Key
	}
	if ev.Service != "" {
		name = ev.Service + "/" + name
	}
	line := fmt.Sprintf("%s  %-26s %-8s", ev.Timestamp, name, ev.Status)
	if ev.DurationMS > 0 {
		line += fmt.Sprintf(" %8s", (time.Duration(ev.DurationMS) * time.Millisecond).Round(time.Millisecond))
	}
	if ev.Reason != "" {
		line += " [" + ev.Reason + "]"
	}
	if ev.Detail != "" {
		line += " " + ev.Detail
	}
	return line
}

// runAppsDeploysSteps implements "apps deploys steps <name> <deploy-id>".
func runAppsDeploysSteps(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys steps", "print one JSON object per step event instead of text", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysStepsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys steps", "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	name, deployID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	failed := false
	var grace *time.Timer
	defer func() {
		if grace != nil {
			grace.Stop()
		}
	}()
	err := client.StreamDeploySteps(ctx, name, deployID, func(ev deployStepEvent) error {
		if jsonOut {
			if err := writeJSONLine(stdout, ev); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintln(stdout, formatStepLine(ev)); err != nil {
			return err
		}
		if final, bad := deployStepsFinal(ev); final {
			failed = bad
			return errStepsDone
		}
		if ev.Step == "deploying" && ev.Status == "done" && grace == nil {
			grace = time.AfterFunc(stepsGrace, cancel)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStepsDone) && !errors.Is(err, context.Canceled) {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stream deploy steps for %s/%s: %w", name, deployID, err))
	}
	if failed {
		return exitAPIError
	}
	return exitOK
}

func appsDeploysStepsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys steps <name> <deploy-id> [flags]

Streams one deploy attempt's pipeline trace, the same feed the dashboard
shows: for every deploy, build or not, the image resolution, pull, create,
start, readiness probes, cutover and retained image, each with its status,
duration and reason. It exits when the trace's summary step lands: 0 on
success, non-zero on failure. A finished attempt replays its stored trace.
Find a deploy-id with "apps deploys list <name>".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print one JSON object per step event instead of text
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// errStepsDone stops the step stream once the pipeline has ended; the server
// holds the connection open after the last step.
var errStepsDone = errors.New("deploy steps finished")

// deployStepsTerminal reports whether ev is the last step a stream carries:
// any failure, or the final deploying step completing.
func deployStepsTerminal(ev deployStepEvent) bool {
	return ev.Status == "failed" || (ev.Step == "deploying" && ev.Status == "done")
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

	failed := false
	err := client.StreamDeploySteps(ctx, name, deployID, func(ev deployStepEvent) error {
		if jsonOut {
			if err := writeJSONLine(stdout, ev); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(stdout, "%s  %-10s %s\n", ev.Timestamp, ev.Step, ev.Status); err != nil {
			return err
		}
		if ev.Status == "failed" {
			failed = true
		}
		if deployStepsTerminal(ev) {
			return errStepsDone
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

Streams one deploy attempt's pipeline steps (detecting, building,
pushing, deploying), the same feed the dashboard shows. It prints each
step transition as it happens and exits when the pipeline ends: 0 on
success, non-zero if a step failed. For an attempt that already
finished, only a short two-point summary is available. Find a deploy-id
with "apps deploys list <name>".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print one JSON object per step event instead of text
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

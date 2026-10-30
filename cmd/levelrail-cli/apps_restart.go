package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const envRestartWaitTimeout = "APP_RESTART_WAIT_TIMEOUT"

// Recreates the container without an image change, since a plain redeploy of the same tag is a no-op.
func runAppsRestart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps restart", "print the app as JSON to stdout and nothing else", stderr)
	var queueOffline bool
	fs.BoolVar(&queueOffline, "queue-while-offline", false, "if the app's node is offline, record the change as pending and apply it when the node returns (default: refuse)")
	var wait bool
	var timeoutStr string
	fs.BoolVar(&wait, "wait", false, "wait for the new container to pass readiness and exit non-zero if the restart fails")
	fs.StringVar(&timeoutStr, "timeout", "", "with --wait, give up after this long (default "+envRestartWaitTimeout+", then 10m)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsRestartUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps restart requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	timeout := envDurationOr(lookupEnv, envRestartWaitTimeout, 10*time.Minute)
	if timeoutStr != "" {
		d, err := parseDurationOrZero(timeoutStr)
		if err != nil || d <= 0 {
			return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must be a valid positive duration, got %q", timeoutStr))
		}
		timeout = d
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	restarted, err := appAction(context.Background(), client, name, "restart", queueOffline, func() (apiclient.AppResource, error) { return client.RestartApp(context.Background(), name) })
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restart app %q: %w", name, err))
	}
	if wait {
		return waitForAppRestart(client, prog, name, timeout, of, stdout, stderr, jsonOut)
	}

	if err := renderResult(stdout, of.Format, of.Query, restarted, func() {
		_, _ = fmt.Fprintf(stderr, "app %q restart requested; rolling restart in progress, follow it with \"%s apps deploys wait %s\"\n", restarted.Name, prog, restarted.Name)
		printAppHuman(stdout, restarted)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// waitForAppRestart follows the restart's own attempt (the newest one) so the
// exit code is the restart's real outcome, not the request's acceptance.
func waitForAppRestart(client *Client, prog, name string, timeout time.Duration, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	if of.Format != outputJSON && of.Query == "" {
		_, _ = fmt.Fprintf(stderr, "app %q restart requested; waiting up to %s for the new container to pass readiness\n", name, timeout)
	}
	res, err := client.WaitForDeploy(context.Background(), name, "latest", apiclient.WaitOptions{Window: timeout})
	if err != nil && !errors.Is(err, context.Canceled) {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("wait for restart of app %q: %w", name, err))
	}
	out := deploysWaitResult{App: name, DeployID: res.Deploy.ID, Status: res.Deploy.Outcome, Failure: res.Deploy.Failure, Deploy: res.Deploy}
	if !res.Done {
		out.Status = "timeout"
	}
	code := writeScheduledTaskResult(stdout, stderr, of, out, func() { printDeploysWaitHuman(stdout, out, timeout.String()) })
	switch {
	case code != exitOK:
		return code
	case !res.Done:
		_, _ = fmt.Fprintf(stderr, "%s: the restart is still rolling out; follow it with \"%s apps deploys wait %s\"\n", prog, prog, name)
		return exitDeployTimeout
	case out.Status == apiclient.DeployOutcomeHealthy:
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: the restart of %q failed; the previous container keeps serving if it was healthy\n", prog, name)
		return exitDeployNotHealthy
	}
}

func appsRestartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps restart <name> [flags]

Forces an existing app's running container to be recreated with no image
change. Different from "apps deploy" and "apps rollback": neither of
those force a restart when the image is unchanged; this command always
does, useful when a process needs a fresh start without a new build.

The restart is rolling: a new container must pass readiness before traffic
switches, and the old one is kept for a while. It also picks up saved env
changes. It is recorded as a deploy with trigger "restart", so "apps deploys
list" and "apps deploys wait" cover it. A stopped app is refused: run
"apps start" first. "%[1]s restart <name>" is the same command.

With --wait the command follows that deploy and exits 0 only when the new
container is healthy: a failed restart exits non-zero, and a timeout exits
with the deploy timeout code. With --json it prints the wait result.

Flags:
  --wait                  wait for the restart's real outcome before exiting
  --timeout duration      with --wait, how long to wait (default %[5]s env var, then 10m)
  --queue-while-offline   if the app's node is offline, record the change as pending (default: refuse)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, envRestartWaitTimeout)
}

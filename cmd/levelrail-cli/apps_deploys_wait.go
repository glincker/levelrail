package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// exitDeployNotHealthy is "apps deploys wait" finishing with a deploy that
// failed, was canceled, superseded or blocked.
const exitDeployNotHealthy = 7

type deploysWaitResult struct {
	App      string                          `json:"app"`
	DeployID string                          `json:"deploy_id"`
	Status   string                          `json:"status"`
	Failure  *apiclient.DeployFailure        `json:"failure,omitempty"`
	Deploy   apiclient.DeployAttemptResource `json:"deploy"`
}

// runAppsDeploysWait implements "apps deploys wait <name> [deploy-id]".
func runAppsDeploysWait(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys wait", "print the final result as JSON to stdout and nothing else", stderr)
	var timeoutStr, pollStr string
	fs.StringVar(&timeoutStr, "timeout", "10m", "give up and exit non-zero after this long")
	fs.StringVar(&pollStr, "poll-interval", "2s", "how often to re-check")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysWaitUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys wait requires an app name and an optional deploy id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, deployID := rest[0], "latest"
	if len(rest) == 2 {
		deployID = rest[1]
	}
	timeout, err := parseDurationOrZero(timeoutStr)
	if err != nil || timeout <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--timeout must be a valid positive duration, got %q", timeoutStr))
	}
	poll, err := parseDurationOrZero(pollStr)
	if err != nil || poll <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--poll-interval must be a valid positive duration, got %q", pollStr))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.WaitForDeploy(context.Background(), name, deployID, apiclient.WaitOptions{Interval: poll, Window: timeout})
	if err != nil && !errors.Is(err, context.Canceled) {
		return reportError(stdout, stderr, jsonOut, err)
	}

	out := deploysWaitResult{App: name, DeployID: res.Deploy.ID, Status: res.Deploy.Outcome, Failure: res.Deploy.Failure, Deploy: res.Deploy}
	if !res.Done {
		out.Status = "timeout"
	}
	code := writeScheduledTaskResult(stdout, stderr, of, out, func() { printDeploysWaitHuman(stdout, out, timeout.String()) })
	if code != exitOK {
		return code
	}
	switch {
	case !res.Done:
		return exitDeployTimeout
	case out.Status == apiclient.DeployOutcomeHealthy:
		return exitOK
	default:
		return exitDeployNotHealthy
	}
}

func printDeploysWaitHuman(w io.Writer, r deploysWaitResult, timeout string) {
	if r.Status == "timeout" {
		_, _ = fmt.Fprintf(w, "timed out after %s waiting for deploy %s of %q\n", timeout, r.DeployID, r.App)
		return
	}
	_, _ = fmt.Fprintf(w, "deploy %s of %q: %s\n", r.DeployID, r.App, r.Status)
	if r.Failure != nil {
		printDeployFailure(w, r.Failure)
	}
}

func appsDeploysWaitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys wait <name> [deploy-id] [flags]

Blocks until one deploy is healthy, failed, canceled, superseded or blocked
(a freeze window or approval), then prints the final status and, when it did
not succeed, the structured failure. Without deploy-id the newest deploy is
resolved once and then followed by id, so a later deploy or restart cannot
change the result.

Exit codes: 0 healthy, %[5]d failed, canceled, superseded or blocked, %[6]d
timeout.

Flags:
  --timeout string          give up after this long (Go duration, default 10m)
  --poll-interval string    how often to re-check (Go duration, default 2s)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the final result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, exitDeployNotHealthy, exitDeployTimeout)
}

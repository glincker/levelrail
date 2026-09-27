package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsDeploysShow implements "apps deploys show <name> [deploy-id]".
func runAppsDeploysShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys show", "print the deploy attempt as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysShowUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys show requires an app name and an optional deploy id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, deployID := rest[0], "latest"
	if len(rest) == 2 {
		deployID = rest[1]
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	attempt, err := client.GetDeploy(context.Background(), name, deployID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get deploy %q for app %q: %w", deployID, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, attempt, func() { printDeployAttemptDetail(stdout, attempt) })
}

func printDeployAttemptDetail(w io.Writer, a deployAttemptResource) {
	_, _ = fmt.Fprintf(w, "deploy:   %s\napp:      %s\nstatus:   %s\nimage:    %s\n", a.ID, a.ServiceName, a.Status, unpinnedImage(a.Image))
	if a.CommitSHA != "" {
		_, _ = fmt.Fprintf(w, "commit:   %s\n", a.CommitSHA)
	}
	_, _ = fmt.Fprintf(w, "started:  %s\n", a.StartedAt.Format(time.RFC3339))
	if a.FinishedAt != nil {
		_, _ = fmt.Fprintf(w, "finished: %s\n", a.FinishedAt.Format(time.RFC3339))
	}
	if a.Failure != nil {
		_, _ = fmt.Fprintln(w)
		printDeployFailure(w, a.Failure)
	}
}

// printDeployFailure renders the shared failure object for humans.
func printDeployFailure(w io.Writer, f *apiclient.DeployFailure) {
	_, _ = fmt.Fprintf(w, "failure:  %s (retryable: %t)\n  cause:  %s\n", f.Code, f.Retryable, f.Cause)
	if f.FailingStep != "" {
		_, _ = fmt.Fprintf(w, "  step:   %s\n", f.FailingStep)
	}
	_, _ = fmt.Fprintf(w, "  fix:    %s\n  docs:   %s\n", f.SuggestedFix, f.DocsURL)
	if f.LogExcerpt != "" {
		_, _ = fmt.Fprintf(w, "  log excerpt:\n%s\n", indentLines(f.LogExcerpt, "    "))
	}
}

func appsDeploysShowUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys show <name> [deploy-id] [flags]

Shows one deploy attempt (the newest when deploy-id is omitted). When the
deploy failed or is blocked, it includes the structured failure object:
code, cause, failing step, a capped and redacted log excerpt, suggested
fix, docs link and whether a retry can help. The same object is returned
by the API and the MCP get_deploy tool.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the deploy attempt as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

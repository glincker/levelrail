package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsHookRuns implements "apps hook-runs <name>": the most recent
// outcome of each of name's pre/post-deploy hooks (GET
// /api/v1/apps/{name}/hook-runs, internal/api/apps_hooks.go), the
// deploy-history-adjacent view this feature's own CLAUDE.md completeness
// rule asks for: a pre-deploy hook failure already shows up in "apps
// status" as a PreDeployHookFailed condition, this command is where an
// operator goes to see exactly what the hook printed.
func runAppsHookRuns(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps hook-runs", "print the hook runs as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps hook-runs <name> [flags]\n\nShows the most recent outcome of name's pre-deploy and post-deploy\nhook commands (app.yaml's hooks: block), including their output.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps hook-runs", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	runs, err := client.GetAppHookRuns(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get hook runs for %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, runs, func() { printAppHookRunsHuman(stdout, runs) })
}

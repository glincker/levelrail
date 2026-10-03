package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsHealthScore implements "apps health-score <name>": GET
// /api/v1/apps/{name}/health-score, the same pass/warn/fail synthesis
// AppHealthScorePanel renders. A separate command from "apps health"
// (apps_health.go, probe get/set/clear) rather than a new verb under
// it: a different resource entirely, a computed verdict not stored
// probe config, same "different shape, different command" reasoning
// as apps_status.go vs "apps get".
func runAppsHealthScore(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps health-score", "print the health score as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps health-score <name> [flags]\n\nShows a synthesized pass/warn/fail readiness verdict for one app, across\ndeploy health, security, resilience, and observability.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps health-score", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	score, err := client.GetAppHealthScore(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get health score for app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, score, func() { printAppHealthScoreHuman(stdout, score) })
}

// printAppHealthScoreHuman renders score as a plain-text table: overall
// verdict first, then one line per category, widest label first so the
// status column lines up.
func printAppHealthScoreHuman(out io.Writer, score appHealthScoreResource) {
	_, _ = fmt.Fprintf(out, "%s: %s\n\n", score.AppName, score.Status)
	width := 0
	for _, c := range score.Categories {
		if len(c.Label) > width {
			width = len(c.Label)
		}
	}
	for _, c := range score.Categories {
		_, _ = fmt.Fprintf(out, "%-*s  %-5s  %s\n", width, c.Label, c.Status, c.Reason)
	}
}

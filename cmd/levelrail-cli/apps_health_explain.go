package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsHealthExplain is "apps health explain <name>": why an app is red or amber.
func runAppsHealthExplain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps health explain", "print the explanation as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps health explain <name> [flags]\n\nShows the last failing check, restarts, last deploy, a recent error line and the suggested next step.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps health explain", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.GetAppHealthExplain(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("explain health for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printHealthExplainHuman(stdout, res) })
}

func printHealthExplainHuman(out io.Writer, r apiclient.AppHealthExplainResource) {
	_, _ = fmt.Fprintf(out, "%s: %s\n", r.AppName, r.State)
	if c := r.FailingCheck; c != nil {
		detail := c.Error
		if c.StatusCode != 0 {
			detail = fmt.Sprintf("status %d %s", c.StatusCode, c.Error)
		}
		_, _ = fmt.Fprintf(out, "failing check: %s (%s) since %s\n", c.Target, detail, c.Since.Format(time.RFC3339))
	}
	if r.Restarts != nil {
		note := ""
		if r.Crashloop {
			note = " (crashloop)"
		}
		_, _ = fmt.Fprintf(out, "restarts: %d%s\n", *r.Restarts, note)
	} else if r.Crashloop {
		_, _ = fmt.Fprintln(out, "restarts: crashloop firing")
	}
	if d := r.LastDeploy; d != nil {
		_, _ = fmt.Fprintf(out, "last deploy: %s %s at %s\n", d.Image, d.Status, d.StartedAt.Format(time.RFC3339))
	}
	if r.LogError != "" {
		_, _ = fmt.Fprintf(out, "recent error: %s\n", r.LogError)
	}
	if r.NextStep.Command != "" {
		_, _ = fmt.Fprintf(out, "next: %s\n", r.NextStep.Command)
	}
}

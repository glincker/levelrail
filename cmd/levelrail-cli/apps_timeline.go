package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const defaultTimelineLimit = 20

// runAppsTimeline implements "apps timeline <name>": config and lifecycle
// events merged with deploys, newest first.
func runAppsTimeline(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps timeline", "print the timeline as JSON to stdout and nothing else", stderr)
	var limit int
	fs.IntVar(&limit, "limit", defaultTimelineLimit, "how many entries to show, newest first")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps timeline <name> [--limit N] [flags]\n\nShows what happened to an app, newest first: deploys, rollbacks, restarts,\nenv, secret and config changes (key names only, never values), scaling,\nstop and start.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps timeline", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if limit < 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--limit must be at least 1"))
	}
	resp, err := client.GetAppTimeline(context.Background(), name, limit, "")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get timeline for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, resp, func() { printTimelineHuman(stdout, resp) })
}

func printTimelineHuman(out io.Writer, resp apiclient.TimelineResponse) {
	if len(resp.Items) == 0 {
		_, _ = fmt.Fprintln(out, "no events recorded yet")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WHEN\tKIND\tSTATUS\tACTOR\tWHAT")
	for _, it := range resp.Items {
		what := it.Title
		if it.Detail != "" {
			what += " (" + it.Detail + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", formatTimelineTime(it.At), it.Kind, it.Status, it.Actor, what)
	}
	_ = tw.Flush()
	if resp.NextCursor != "" {
		_, _ = fmt.Fprintln(out, "more entries exist, raise --limit to see them")
	}
}

func formatTimelineTime(at string) string {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return at
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

// printConfigRevisions lists the revisions newest first with a one-line diff summary.
func printConfigRevisions(out io.Writer, revs []apiclient.ConfigRevision) {
	if len(revs) == 0 {
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "REV\tWHEN\tACTOR\tCHANGED")
	for _, r := range revs {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", r.N, r.At.Local().Format("2006-01-02 15:04:05"), r.Actor, summarizeConfigChanges(r))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(out)
}

func summarizeConfigChanges(r apiclient.ConfigRevision) string {
	if len(r.Changes) == 0 {
		return "first recorded state"
	}
	parts := make([]string, 0, len(r.Changes))
	for _, c := range r.Changes {
		parts = append(parts, c.Kind+" "+c.Field)
	}
	s := strings.Join(parts, ", ")
	if r.RevertOf > 0 {
		s += fmt.Sprintf(" (revert to %d)", r.RevertOf)
	}
	return s
}

func printConfigDiff(out io.Writer, changes []apiclient.ConfigFieldChange) {
	for _, c := range changes {
		_, _ = fmt.Fprintf(out, "  %s %s: %s -> %s\n", c.Kind, c.Field, revDash(c.From), revDash(c.To))
	}
}

func revDash(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// runAppsRevert implements "apps revert <name> [--to N] [--yes]".
func runAppsRevert(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps revert", "print the result as JSON to stdout and nothing else", stderr)
	var to int
	var yes bool
	fs.IntVar(&to, "to", 0, "revision number to return to (default: the one before the latest)")
	fs.BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps revert <name> [--to N] [--yes] [flags]\n\nApplies the env, scale, resources, domains and health of an earlier revision\nas a new revision. Database data, volumes and the image are not touched, and\nsecret values are not restored from history. Env changes apply on the next\nrestart or deploy. See \"apps changes\" for revision numbers.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps revert", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	if to == 0 {
		res, err := client.GetAppChanges(ctx, name, time.Time{}, "")
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get changes for app %q: %w", name, err))
		}
		if len(res.Revisions) < 2 {
			return reportError(stdout, stderr, jsonOut, newValidationError("no earlier revision to return to, run %s apps changes %s", prog, name))
		}
		to = res.Revisions[1].N
	}
	preview, err := client.RevertApp(ctx, name, to, true)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revert app %q: %w", name, err))
	}
	if len(preview.Changes) == 0 {
		return writeScheduledTaskResult(stdout, stderr, of, preview, func() { _, _ = fmt.Fprintf(stdout, "%s already matches revision %d\n", name, to) })
	}
	if !yes {
		if !jsonOut {
			_, _ = fmt.Fprintf(stderr, "Reverting %s to revision %d changes:\n", name, to)
			printConfigDiff(stderr, preview.Changes)
			_, _ = fmt.Fprintln(stderr, "Only app settings change. Database data and volumes are not touched, and secret values are not restored.")
		}
		if err := confirmDestructive(stderr, fmt.Sprintf("Revert %s to revision %d?", name, to)); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
	}
	res, err := client.RevertApp(ctx, name, to, false)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revert app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "reverted %s to revision %d (new revision %d)\n", name, to, res.Revision)
		if res.PendingApply {
			_, _ = fmt.Fprintln(stdout, "env changes apply on the next restart or deploy")
		}
		if len(res.Skipped) > 0 {
			_, _ = fmt.Fprintf(stdout, "not restored (secret values are never kept in history): %s\n", strings.Join(res.Skipped, ", "))
		}
	})
}

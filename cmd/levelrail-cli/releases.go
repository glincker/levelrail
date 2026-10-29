package main

import (
	"context"
	"fmt"
	"io"
)

// runReleases dispatches `releases list <app>` and `releases history <app>`,
// the read-only views of an app's release snapshots and binding history.
func runReleases(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "list" && args[0] != "history") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s releases list <app> [flags]\n  %s releases history <app> [flags]\n", prog, prog)
		return exitUsage
	}
	history := args[0] == "history"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "releases "+args[0], "print the releases as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s releases list <app> [flags]\n\nLists the app's release snapshots, newest first. The list is empty\nunless APP_RELEASE_BINDINGS is shadow or on.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "releases " + args[0], "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ListAppReleases(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list releases for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if history {
			if len(res.Bindings) == 0 {
				_, _ = fmt.Fprintf(stdout, "no binding history recorded for %s\n", name)
				return
			}
			_, _ = fmt.Fprintf(stdout, "%-30s %-22s %-12s %s\n", "RELEASE", "AT", "REASON", "ACTOR")
			for _, b := range res.Bindings {
				_, _ = fmt.Fprintf(stdout, "%-30s %-22s %-12s %s\n", b.ReleaseID, b.At, b.Reason, b.Actor)
			}
			return
		}
		if len(res.Releases) == 0 {
			_, _ = fmt.Fprintf(stdout, "no releases recorded for %s\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%-30s %-20s %-22s %-24s %s\n", "RELEASE", "DIGEST", "CREATED", "REASON", "CURRENT")
		for _, r := range res.Releases {
			cur := ""
			if r.ID == res.CurrentReleaseID {
				cur = "*"
			}
			_, _ = fmt.Fprintf(stdout, "%-30s %-20s %-22s %-24s %s\n", r.ID, r.ShortDigest, r.CreatedAt, r.SourceReason, cur)
		}
	})
}

package main

import (
	"context"
	"fmt"
	"io"
)

// runReleases dispatches `releases list <app>`, the read-only view of an
// app's release snapshots.
func runReleases(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "list" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s releases list <app> [flags]\n", prog)
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "releases list", "print the releases as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s releases list <app> [flags]\n\nLists the app's release snapshots, newest first. The list is empty\nunless APP_RELEASE_BINDINGS is shadow or on.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "releases list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.ListAppReleases(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list releases for app %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
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

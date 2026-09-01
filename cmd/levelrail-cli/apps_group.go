package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsGroup implements "apps group <name>": name's sibling services under
// the same store.App plus one rollup status, whether or not siblings exist yet.
func runAppsGroup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps group", "print the app group as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps group <name> [flags]\n\nShows name's sibling services under the same multi-service app, plus a\nrollup status across all of them.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP}, stderr, singleArgCmd{prog, "apps group", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	group, err := client.GetAppGroup(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app group for %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, jsonOut, group, func() { printAppGroupHuman(stdout, group) })
}

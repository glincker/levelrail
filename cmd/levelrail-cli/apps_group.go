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

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps group", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	group, err := client.GetAppGroup(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app group for %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, group); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printAppGroupHuman(stdout, group)
	return exitOK
}

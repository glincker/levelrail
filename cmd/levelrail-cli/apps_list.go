package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsList implements "apps list": GET /api/v1/apps, printed either
// as a compact table or as JSON. Kept deliberately small (the task this
// command exists for is verifying "apps create" worked, not being a
// full-featured resource browser), per this CLI's own scope: apps
// create is the point, list/get are minimal companions.
func runAppsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "apps list", "print apps as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps list [flags]\n\nLists every app the caller's token can read.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	apps, err := client.ListApps(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, apps); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printAppsTable(stdout, apps)
	return exitOK
}

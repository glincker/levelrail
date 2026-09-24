package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesStatus implements "databases status <name>": the database
// controller's current stored reconcile conditions.
func runDatabasesStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases status", "print conditions as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases status <name> [flags]\n\nShows the database controller's current stored reconcile conditions\n(current status, not a history log). Useful when a database exists but\nis not running yet.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases status", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	conditions, err := client.GetDatabaseStatus(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get status for database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, conditions, func() { printConditionsHuman(stdout, conditions) })
}

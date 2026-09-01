package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesGet implements "databases get <name>": GET
// /api/v1/databases/{name}.
func runDatabasesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases get", "print the database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases get <name> [flags]\n\nShows one managed database's current desired state.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "databases get", "database name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	db, err := client.GetDatabase(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, db, func() { printDatabaseHuman(stdout, db) })
}

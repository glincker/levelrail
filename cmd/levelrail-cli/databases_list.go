package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesList implements "databases list": GET /api/v1/databases.
func runDatabasesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases list", "print databases as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases list [flags]\n\nLists every managed database the caller's token can read.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	dbs, err := client.ListDatabases(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list databases: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, dbs, func() { printDatabasesTable(stdout, dbs) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

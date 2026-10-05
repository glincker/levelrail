package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesSetVersion implements "databases set-version <name> <version>":
// PUT /api/v1/databases/{name}/version, a minor or patch image change over
// the same data volume.
func runDatabasesSetVersion(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases set-version", "print the updated database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesSetVersionUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: databases set-version requires a database name and a version\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, version := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetDatabaseVersion(context.Background(), name, version)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set version for database %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "database %q will be recreated on version %q over the same data volume\n", name, updated.Version)
	})
}

func databasesSetVersionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases set-version <name> <version> [flags]

Changes a database's image tag, for a minor or patch update (16 to 16.4).
The container is recreated over the same data volume. A major version change
is refused because the data files are not portable: take a backup and restore
it into a new database instead. Take a backup first.

`, prog) + databaseMoveNodeFlagsUsage()
}

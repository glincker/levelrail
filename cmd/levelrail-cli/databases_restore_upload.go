package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// runDatabasesRestoreUpload implements "databases restore-upload <name>
// <file-path>": POST /api/v1/databases/{name}/restore-upload
// (internal/api/database_restore_upload.go), restoring name from a dump
// file on this machine rather than a backup already stored on the
// control plane. The file is streamed straight from disk into the
// request, not read fully into memory first: a database dump can be
// gigabytes.
func runDatabasesRestoreUpload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases restore-upload", "print the restore outcome as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesRestoreUploadUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "databases restore-upload", "a database name and a dump file path", 2)
	if !ok {
		return exitUsage
	}
	name, filePath := rest[0], rest[1]

	f, err := os.Open(filePath) //nolint:gosec // operator-supplied CLI argument, the same pattern apps_deploy_compose.go's own --file read uses
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("open %s: %w", filePath, err))
	}
	defer func() { _ = f.Close() }()

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.RestoreDatabaseFromReader(context.Background(), name, f)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restore database %q from %s: %w", name, filePath, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "database %q restored from %s (status: %s)\n", name, filePath, result.Status)
	})
}

func databasesRestoreUploadUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases restore-upload <name> <file-path> [flags]

Restores <name> from a dump file already on this machine, overwriting
its current data in place. Unlike "%[1]s databases restore" this does
not read from a backup this platform already took: use it to migrate an
existing external database in for the first time, or to apply a
manually-taken dump. Blocks until the restore actually finishes.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the restore outcome as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

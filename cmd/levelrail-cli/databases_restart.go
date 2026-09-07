package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesRestart implements "databases restart <name>": POST
// /api/v1/databases/{name}/restart (internal/api/database_restart.go's
// own handleRestartDatabase). Stops then starts the database's current
// container in place, synchronously: unlike "apps restart", this
// command waits for the actual restart to finish before returning,
// since a database container is never recreated under a new name the
// way an app's is (see handleRestartDatabase's own doc comment).
func runDatabasesRestart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases restart", "print the database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesRestartUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases restart", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	restarted, err := client.RestartDatabase(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("restart database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, restarted, func() {
		_, _ = fmt.Fprintf(stdout, "database %q restarted\n", name)
	})
}

func databasesRestartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases restart <name> [flags]

Stops then starts the database's current container in place. Useful when
the engine process needs a fresh start without changing its image or
losing its data volume.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the database as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

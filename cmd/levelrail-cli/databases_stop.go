package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesStop implements "databases stop <name>": POST
// /api/v1/databases/{name}/stop (internal/api/database_stop_start.go's
// own handleStopDatabase), no request body. Marks the database
// suspended so the reconciler removes its container on the next pass,
// without touching its data volume; "databases start" clears the flag
// again.
func runDatabasesStop(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases stop", "print the database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesStopUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases stop", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	stopped, err := client.StopDatabase(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("stop database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, stopped, func() {
		_, _ = fmt.Fprintf(stderr, "database %q stop requested; reconcile is asynchronous\n", stopped.Name)
		printDatabaseHuman(stdout, stopped)
	})
}

func databasesStopUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases stop <name> [flags]

Stops a database's running container without changing its desired
state or its data volume. Reversed with "%[1]s databases start <name>".

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

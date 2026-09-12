package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesStart implements "databases start <name>": POST
// /api/v1/databases/{name}/start (internal/api/database_stop_start.go's
// own handleStartDatabase), no request body. Clears the suspended flag
// "databases stop" set, letting the reconciler recreate the container
// against the same data volume it always used.
func runDatabasesStart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases start", "print the database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesStartUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases start", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	started, err := client.StartDatabase(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stderr, "database %q start requested; reconcile is asynchronous\n", started.Name)
		printDatabaseHuman(stdout, started)
	})
}

func databasesStartUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases start <name> [flags]

Clears a stopped database's suspended flag, letting the reconciler bring
its container back against the same data volume.

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

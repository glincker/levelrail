package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesSetProject implements "databases set-project <name>
// <project-id>": PUT /api/v1/databases/{name}/project, the database
// counterpart to runAppsSetProject.
func runDatabasesSetProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases set-project", "print the updated database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesSetProjectUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "databases set-project", "a database name and a project id", 2)
	if !ok {
		return exitUsage
	}
	name, projectID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetDatabaseProject(context.Background(), name, projectID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set project for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "database %q moved into project %q\n", name, projectID)
	})
}

func databasesSetProjectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases set-project <name> <project-id> [flags]

Moves a database into a project (create one first via "%[1]s apps
projects create"). Purely organizational, doesn't change how the
database runs.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated database as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runDatabasesClearProject implements "databases clear-project <name>":
// PUT /api/v1/databases/{name}/project with an empty project_id.
func runDatabasesClearProject(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases clear-project", "print the updated database as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesClearProjectUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases clear-project", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.SetDatabaseProject(context.Background(), name, "")
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear project for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "database %q removed from its project\n", name)
	})
}

func databasesClearProjectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases clear-project <name> [flags]

Removes a database's project assignment.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated database as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

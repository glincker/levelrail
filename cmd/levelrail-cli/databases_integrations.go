package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesIntegrations dispatches "databases integrations list <database>".
func runDatabasesIntegrations(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesIntegrationsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesIntegrationsUsage(prog))
		return exitOK
	case "list", "ls":
		return runDatabasesIntegrationsList(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases integrations subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesIntegrationsUsage(prog))
		return exitUsage
	}
}

func databasesIntegrationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases integrations list <database> [flags]

Lists the integration catalog for a database with each card's state:
connected, available, needs_setup, docs_only, unavailable or planned.
`, prog)
}

func runDatabasesIntegrationsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := "databases integrations list"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the catalog as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesIntegrationsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, cmd, "database name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ListDatabaseIntegrations(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for database %q: %w", cmd, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		for _, c := range res.Integrations {
			_, _ = fmt.Fprintf(stdout, "%-20s %-10s %-12s %s\n", c.ID, c.Category, c.State.Code, c.State.Reason)
		}
	})
}

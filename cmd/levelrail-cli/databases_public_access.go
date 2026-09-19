package main

import (
	"context"
	"fmt"
	"io"
)

// runDatabasesPublicAccess dispatches "databases public-access <verb>
// <database> [flags]" to one of set/clear, mirroring runBackupsSchedule's
// own set/clear dispatch shape for a different per-database sub-config.
func runDatabasesPublicAccess(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesPublicAccessUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesPublicAccessUsage(prog))
		return exitOK
	case "set":
		return runDatabasesPublicAccessSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDatabasesPublicAccessClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases public-access subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesPublicAccessUsage(prog))
		return exitUsage
	}
}

func databasesPublicAccessUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases public-access set <database> [flags]     expose a database on a host port
  %[1]s databases public-access clear <database> [flags]   return a database to internal-network-only

Run "%[1]s databases public-access <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runDatabasesPublicAccessSet implements "databases public-access set
// <database>": PUT /api/v1/databases/{name}/public-access
// (internal/api/database_public_access.go). --bind-address chooses which
// network interface the assigned port binds to; omitted, the control
// plane defaults to "private" (loopback only), never a silent "public".
func runDatabasesPublicAccessSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases public-access set", "print the updated public-access state as JSON to stdout and nothing else", stderr)
	var port int
	var bindAddress string
	fs.IntVar(&port, "port", 0, "host port to expose on (default: auto-assigned by the control plane)")
	fs.StringVar(&bindAddress, "bind-address", "", "network interface the port binds to: \"private\" (loopback only, the default), \"public\" (every interface), or a literal IP")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases public-access set <database> [flags]\n\nExposes <database> on a host port so a database GUI tool (pgAdmin,\nTablePlus, RedisInsight) can connect from your own machine, not just\nfrom inside the Docker network. Replaces any previously configured\npublic-access state for <database>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases public-access set", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.SetDatabasePublicAccess(context.Background(), name, port, bindAddress)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set public access for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "database %q publicly accessible on port %d, bound to %q\n", name, result.PublicPort, result.BindAddress)
	})
}

// runDatabasesPublicAccessClear implements "databases public-access
// clear <database>": DELETE /api/v1/databases/{name}/public-access.
func runDatabasesPublicAccessClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases public-access clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases public-access clear <database> [flags]\n\nReturns <database> to internal-network-only: no host port bound.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases public-access clear", "database name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.ClearDatabasePublicAccess(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear public access for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "public access removed for database %q\n", name)
	})
}

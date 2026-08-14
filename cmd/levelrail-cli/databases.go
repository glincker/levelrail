package main

import (
	"fmt"
	"io"
)

// runDatabases dispatches "databases <verb> [flags]" to one of
// create/list/get, the same three-verb shape "apps" started with (see
// apps.go's own doc comment on why create/list/get is the minimal set:
// create is the point, list/get are companions for verifying it worked).
func runDatabases(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesUsage(prog))
		return exitOK
	case "create":
		return runDatabasesCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runDatabasesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runDatabasesGet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesUsage(prog))
		return exitUsage
	}
}

func databasesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases create [flags]     create a managed database
  %[1]s databases list [flags]         list databases
  %[1]s databases get <name> [flags]   show one database

Run "%[1]s databases <subcommand> -h" for a subcommand's own flags.
`, prog)
}

package main

import (
	"fmt"
	"io"
	"os"
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
		return runDatabasesCreate(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "list":
		return runDatabasesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runDatabasesGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runDatabasesDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "resource-recommendation":
		return runDatabasesResourceRecommendation(prog, args[1:], stdout, stderr, lookupEnv)
	case "set-project":
		return runDatabasesSetProject(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear-project":
		return runDatabasesClearProject(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesUsage(prog))
		return exitUsage
	}
}

func databasesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases create [flags]     create a managed database
  %[1]s databases create --interactive  guided, step-by-step creation
  %[1]s databases list [flags]         list databases
  %[1]s databases get <name> [flags]   show one database
  %[1]s databases delete <name> [flags]  remove a database's desired state
  %[1]s databases resource-recommendation <name> [flags]  suggest memory/CPU limits from historical usage
  %[1]s databases set-project <name> <project-id> [flags]  move a database into a project
  %[1]s databases clear-project <name> [flags]  remove a database's project assignment

Run "%[1]s databases <subcommand> -h" for a subcommand's own flags.
`, prog)
}

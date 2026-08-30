package main

import (
	"fmt"
	"io"
)

// runUsers dispatches "users <verb> [args] [flags]" to one of
// list/create/set-abilities/delete/roles. Every route these subcommands
// call is requireAbility-gated server-side (internal/api/routes.go), not
// session-only like "tokens", so like "nodes" these use the normal
// bearer token resolution every other command in this CLI uses, provided
// the token actually carries the required ability (AbilityRoot for
// create/set-abilities/delete, AbilityRead for list/roles).
func runUsers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usersUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usersUsage(prog))
		return exitOK
	case "list":
		return runUsersList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runUsersCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "set-abilities":
		return runUsersSetAbilities(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runUsersDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "roles":
		return runUsersRoles(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown users subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, usersUsage(prog))
		return exitUsage
	}
}

func usersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users list [flags]                                          list every user
  %[1]s users create --email EMAIL --password PASSWORD (--role ROLE | --abilities LIST) [flags]   create a user
  %[1]s users set-abilities <id> (--role ROLE | --abilities LIST) [flags]                          replace a user's abilities, directly or via a curated role
  %[1]s users delete <id> [flags]                                    remove a user
  %[1]s users roles [flags]                                          list the curated role presets --role accepts

Run "%[1]s users <subcommand> -h" for a subcommand's own flags.
`, prog)
}

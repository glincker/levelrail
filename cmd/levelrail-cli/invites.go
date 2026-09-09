package main

import (
	"fmt"
	"io"
)

// runInvites dispatches "invites <verb> [args] [flags]" to one of
// create/list/revoke. Every route these subcommands call is
// requireAbility-gated server-side (internal/api/routes.go) at
// AbilityRoot, the same tier "users create" itself requires: an invite
// is a deferred version of that same "hand out access" action.
func runInvites(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, invitesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, invitesUsage(prog))
		return exitOK
	case "create":
		return runInvitesCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runInvitesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke":
		return runInvitesRevoke(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown invites subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, invitesUsage(prog))
		return exitUsage
	}
}

func invitesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s invites create --email EMAIL --role ROLE [flags]   invite a new teammate
  %[1]s invites list [flags]                                list pending invites
  %[1]s invites revoke <id> [flags]                         revoke a pending invite

Run "%[1]s invites <subcommand> -h" for a subcommand's own flags.
`, prog)
}

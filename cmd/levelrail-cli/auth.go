package main

import (
	"fmt"
	"io"
	"os"
)

// runAuth dispatches "auth <verb> [flags]" to one of login/whoami. Both
// subcommands are session-cookie-adjacent in a way none of this CLI's
// other commands are (see auth_login.go and auth_whoami.go's own doc
// comments for the specific, real gaps that follow from that), so they
// get their own top-level command rather than living under an existing
// one.
func runAuth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authUsage(prog))
		return exitOK
	case "login":
		return runAuthLogin(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "whoami":
		return runAuthWhoami(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authUsage(prog))
		return exitUsage
	}
}

func authUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth login [flags]     authenticate and persist a new API token
  %[1]s auth whoami [flags]   show who the current token authenticates as

Run "%[1]s auth <subcommand> -h" for a subcommand's own flags.
`, prog)
}

package main

import (
	"fmt"
	"io"
	"os"
)

// runTokens dispatches create/list/revoke. These routes are session-only, so every subcommand takes --username and --password, not a bearer token.
func runTokens(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, tokensUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, tokensUsage(prog))
		return exitOK
	case "create":
		return runTokensCreate(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "list":
		return runTokensList(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "revoke":
		return runTokensRevoke(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "cleanup":
		return runTokensCleanup(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown tokens subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, tokensUsage(prog))
		return exitUsage
	}
}

func tokensUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens create --name NAME --abilities LIST [flags]   mint a new API token
  %[1]s tokens list [flags]                                       list tokens (never shows a secret)
  %[1]s tokens revoke <id> [flags]                              revoke a token
  %[1]s tokens cleanup [--older-than-days N]                   remove old revoked tokens (root API token)

Every subcommand here requires a username and password (prompted if
omitted): these routes are session-only server-side, a bearer token can
never call them, see "%[1]s tokens create -h" for why.

Run "%[1]s tokens <subcommand> -h" for a subcommand's own flags.
`, prog)
}

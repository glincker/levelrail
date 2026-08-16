package main

import (
	"fmt"
	"io"
	"os"
)

// runTokens dispatches "tokens <verb> [flags]" to one of
// create/list/revoke. Every one of these needs a live session, not a
// bearer token: see tokens_create.go's own doc comment for the specific
// server-side reason (router.go registers every /api/v1/auth/tokens
// route session-only, "a token cannot mint or revoke another token on
// its own behalf"). That is why every subcommand under this one takes
// --username/--password (prompted if omitted) instead of the
// --token/APP_API_TOKEN/credentials-file resolution every other command
// in this CLI uses: there is no bearer token that could ever
// successfully call these routes, scoped however broadly, so offering
// --token here would be an option that can never work.
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

Every subcommand here requires a username and password (prompted if
omitted): these routes are session-only server-side, a bearer token can
never call them, see "%[1]s tokens create -h" for why.

Run "%[1]s tokens <subcommand> -h" for a subcommand's own flags.
`, prog)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// Idempotent server-side, so an already-revoked token reports the same success.
func runTokensRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens revoke", "print a JSON result object to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensRevokeUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	jsonOut := *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: tokens revoke requires exactly one token id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	id := rest[0]

	ctx := context.Background()
	sessionClient, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	if err := sessionClient.RevokeToken(ctx, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke token %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": id, "status": "revoked"}, func() { _, _ = fmt.Fprintf(stdout, "token %q revoked\n", id) })
}

func tokensRevokeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens revoke <id> [flags]

Revokes one API token by id (idempotent: revoking an already-revoked id
also reports success). Requires a live session: --username/--password
(prompted if omitted).

Flags:
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print a JSON result object to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

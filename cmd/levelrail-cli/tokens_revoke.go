package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// runTokensRevoke implements "tokens revoke <id>": DELETE
// /api/v1/auth/tokens/{id}, authenticated with a freshly established
// session, the same reason "tokens create"/"tokens list" are
// (tokens_create.go's own doc comment). Idempotent server-side
// (handleRevokeToken's own doc comment: only 404s for an id that never
// existed, not one already revoked), so this command reports the same
// success for "revoked just now" and "was already revoked."
func runTokensRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" tokens revoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var username, password, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&username, "username", "", "admin username (prompted if omitted)")
	fs.StringVar(&password, "password", "", "admin password (prompted without echo if omitted)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print a JSON result object to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensRevokeUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: tokens revoke requires exactly one token id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	id := rest[0]

	readPassword := func() (string, error) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(b), err
	}
	resolvedUsername, resolvedPassword, err := resolveLoginCredentials(username, password, stdin, stderr, readPassword)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	sessionClient, err := newAuthSessionClient(resolveAPIURL(apiURLFlag, lookupEnv, prog))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	ctx := context.Background()
	if _, err := sessionClient.Login(ctx, resolvedUsername, resolvedPassword); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("log in as %q: %w", resolvedUsername, err))
	}

	if err := sessionClient.RevokeToken(ctx, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke token %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, map[string]string{"id": id, "status": "revoked"}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "token %q revoked\n", id)
	return exitOK
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
  --json                          print a JSON result object to stdout, nothing else
  -h, --help                    show this help
`, prog, envAPIURL, defaultAPIURL)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runTokensCreate implements "tokens create --name NAME --abilities
// LIST": POST /api/v1/auth/tokens (internal/api/tokens.go's own
// handleCreateToken), authenticated with a freshly established session
// (username/password, prompted if omitted), the same reason and the
// same login step "auth login" performs (auth_login.go's own doc
// comment). Unlike "auth login", this command does not write the
// credentials file: it is for minting a token for something else to
// use (a CI job, a scoped MCP credential), not for this CLI's own
// future calls, so nothing about the caller's own local setup should
// change as a side effect of running it.
//
// handleCreateToken's response returns the plaintext token exactly
// once (confirmed by reading that handler directly: createTokenResponse
// embeds tokenResource plus a Token field, and handleListTokens's own
// response type never includes it, matching that handler's own doc
// comment "never returns a token secret... once a token is minted, its
// plaintext is gone from this API's world entirely"). This command
// prints it once, with an explicit note that it will not be shown
// again, the same GitHub PAT convention the server's own doc comments
// describe.
func runTokensCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "tokens create", "print the new token resource as JSON to stdout and nothing else", stderr)
	var name, abilitiesFlag string
	var expiresInDays int
	fs.StringVar(&name, "name", "", "name for the new token (required)")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list, e.g. \"read,deploy\" (required; valid: read, read:sensitive, write, write:sensitive, deploy, root)")
	fs.IntVar(&expiresInDays, "expires-in-days", 0, "token lifetime in days (default: 0, never expires)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, tokensCreateUsage(prog)) }

	if err := fs.Parse(args); err != nil {
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

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	abilities := splitAndTrim(abilitiesFlag)
	if len(abilities) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--abilities is required"))
	}
	if expiresInDays < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--expires-in-days must not be negative"))
	}

	ctx := context.Background()
	sessionClient, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	created, err := sessionClient.CreateToken(ctx, createTokenRequest{Name: name, Abilities: abilities, ExpiresInDays: expiresInDays})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create token %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "token %q (id %s) created\n", created.Name, created.ID)
		_, _ = fmt.Fprintf(stdout, "token value (shown once, not recoverable again): %s\n", created.Token)
	})
}

func tokensCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tokens create --name NAME --abilities LIST [flags]

Mints a new API token, printed once (never recoverable from the server
again after this call). Requires a live session: --username/--password
(prompted if omitted), not this CLI's own persisted token, see
"%[1]s tokens -h" for why.

Flags:
  --name string                 name for the new token (required)
  --abilities string           comma-separated ability list (required; valid: read, read:sensitive, write, write:sensitive, deploy, root)
  --expires-in-days int      token lifetime in days (default: 0, never expires)
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the new token resource as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

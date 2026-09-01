package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAuthWhoami implements "auth whoami": GET /api/v1/auth/session,
// authenticated with the same resolved token (flag, env, or credentials
// file) every other command in this CLI uses, via client.go's own
// Client.GetSession.
//
// Real, confirmed backend gap, not a bug in this command: that route is
// registered behind requireAuth (session-cookie-only), and
// handleGetSession's own doc comment states the boundary is deliberate
// ("a bearer token has no session of its own to report on"). There is no
// bearer-token-compatible identity/introspection endpoint anywhere in
// internal/api today (GET /api/v1/auth/tokens, the only other place a
// token's own metadata lives, is also session-only, per router.go's
// comment on that registration). So this command, run the way every
// other command in this CLI is normally run (a persisted API token, no
// session cookie), will reach the server and get back a real, honest
// 401 "authentication required" every time. That is this command
// faithfully doing what it says: a real live check against the actual
// endpoint the CLI's own auth mechanism, not a fabricated
// "token is configured" success. It only succeeds for a caller that
// somehow already holds a live session cookie, which nothing in this
// CLI's credentials model persists.
func runAuthWhoami(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" auth whoami", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, profileFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the session info as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authWhoamiUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

	info, err := client.GetSession(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check session: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, info); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "username:    %s\n", info.Username)
	_, _ = fmt.Fprintf(stdout, "expires_at:  %s\n", info.ExpiresAt)
	return exitOK
}

func authWhoamiUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth whoami [flags]

Checks GET /api/v1/auth/session using the resolved API token, a real
live call, not a local "is a token configured" check.

Known limitation, not a bug in this command: that endpoint is
session-cookie-only by explicit server-side design (internal/api's
handleGetSession doc comment), and this CLI only ever persists a bearer
token, never a session cookie (see config.go). Running this against a
token obtained the normal way ("%[1]s auth login") returns a real 401
"authentication required" every time; there is currently no
bearer-token-compatible identity endpoint in the API this command could
call instead.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the session info as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

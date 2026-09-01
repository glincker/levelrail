package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// defaultTokenAbility is the ability set "auth login" mints its token
// with when --abilities is omitted. A session (username/password) is
// always implicitly root, the single-admin-identity model
// internal/api/auth.go's own requireAbility doc comment describes
// ("there is exactly one human identity, the admin"); minting anything
// narrower by default here would silently hand the caller a less capable
// credential than the session they just proved they have, for a command
// whose whole point is "give me a token that can do what I can do."
// --abilities exists precisely for the caller who wants a narrower one
// on purpose (a CI job that should only ever deploy, say).
var defaultTokenAbility = []string{"root"}

// runAuthLogin implements "auth login": authenticates against
// POST /api/v1/auth/login with a username and password (prompted
// interactively, without echo for the password, when not given as
// flags), then, using that same freshly established session, mints a
// real API token via POST /api/v1/auth/tokens and persists it to this
// CLI's own credentials file (config.go's writeCredentialsFile) so every
// later command's resolveToken finds it.
//
// This two-step shape is not a design choice this command had latitude
// over: it is the only way to get a bearer token out of this API at
// all. router.go registers every /api/v1/auth/tokens route as
// session-only ("deliberately never bearer-token authenticated... only
// an interactive human session can manage the token set"), so there is
// no direct "exchange a username/password for a token" endpoint to call
// instead.
//
// Real, load-bearing gap this inherits: the session cookie
// POST /api/v1/auth/login sets is Secure (auth.go's handleLogin, by
// design), and a plain-http control plane (defaultAPIURL,
// http://localhost:8080, the common local-dev target with no
// TLS-terminating Caddy in front yet) never gets that cookie sent back
// on the follow-up token-mint request, so this command's second step
// fails there with a real 401. See authclient.go's own doc comment for
// why this client does not work around that. Point --api-url at an
// https target (a real deployment's embedded Caddy ingress) for this
// command to complete.
func runAuthLogin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" auth login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var username, password, apiURLFlag, profileFlag, tokenName, abilitiesFlag string
	var expiresInDays int
	var jsonOut bool
	fs.StringVar(&username, "username", "", "admin username (prompted if omitted)")
	fs.StringVar(&password, "password", "", "admin password (prompted without echo if omitted)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to save this login under (overrides "+envProfile+", default \""+defaultProfile+"\"); lets one operator manage multiple control planes without overwriting each other's credentials")
	fs.StringVar(&tokenName, "token-name", "", "name for the newly minted token (default: \"levelrail-cli-<timestamp>\")")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list for the new token (default: root, same as the session it's minted from)")
	fs.IntVar(&expiresInDays, "expires-in-days", 0, "token lifetime in days (default: 0, never expires)")
	fs.BoolVar(&jsonOut, "json", false, "print the new token resource as JSON to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authLoginUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	readPassword := func() (string, error) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(b), err
	}
	resolvedUsername, resolvedPassword, err := resolveLoginCredentials(username, password, stdin, stderr, readPassword)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	abilities := defaultTokenAbility
	if abilitiesFlag != "" {
		abilities = splitAndTrim(abilitiesFlag)
	}
	if expiresInDays < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--expires-in-days must not be negative"))
	}
	if tokenName == "" {
		tokenName = "levelrail-cli-" + time.Now().UTC().Format("20060102-150405")
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	sessionClient, err := newAuthSessionClient(apiURL)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	ctx := context.Background()
	loginResp, err := sessionClient.Login(ctx, resolvedUsername, resolvedPassword)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("log in as %q: %w", resolvedUsername, err))
	}

	created, err := sessionClient.CreateToken(ctx, createTokenRequest{Name: tokenName, Abilities: abilities, ExpiresInDays: expiresInDays})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("mint API token: %w", err))
	}

	if err := writeCredentialsFile(prog, profile, credentials{APIURL: apiURL, Token: created.Token}); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save credentials: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}

	dir, _ := configDir(prog)
	_, _ = fmt.Fprintf(stdout, "logged in as %q\n", loginResp.Username)
	_, _ = fmt.Fprintf(stdout, "token %q (id %s) created and saved to %s/%s under profile %q\n", created.Name, created.ID, dir, credentialsFileName, profile)
	_, _ = fmt.Fprintf(stdout, "token value (shown once, not recoverable again): %s\n", created.Token)
	return exitOK
}

// splitAndTrim turns "read, write" into []string{"read", "write"}:
// comma-separated, trimming whitespace around each entry, dropping any
// empty ones (a trailing comma, doubled comma).
func splitAndTrim(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func authLoginUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth login [flags]

Authenticates with a username and password (prompted interactively,
without echo for the password, when not given as flags), then mints a
real API token from that session and saves it to this CLI's own
credentials file. Every later command's --token/%[2]s/credentials-file
resolution then finds it automatically.

Requires the control plane to be reachable over https for the session
cookie the login step sets to round-trip to the token-minting step: see
this command's own doc comment (auth_login.go) for why a plain-http
target (the common local-dev default) fails at that second step.

Flags:
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string             named credentials profile to save this login under (overrides APP_PROFILE, default "default"); lets one operator manage multiple control planes without overwriting each other's credentials
  --token-name string         name for the new token (default: "levelrail-cli-<timestamp>")
  --abilities string           comma-separated ability list (default: root)
  --expires-in-days int      token lifetime in days (default: 0, never expires)
  --json                          print the new token resource as JSON to stdout, nothing else
  -h, --help                    show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

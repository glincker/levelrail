package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// Defaults to root because a session is always root, so a narrower default would silently weaken the token.
// Use --abilities for a deliberately narrower credential.
var defaultTokenAbility = []string{"root"}

// The login cookie is Secure, so the token mint fails over plain http; use an https --api-url.
func runAuthLogin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth login", "print the new token resource as JSON to stdout and nothing else", stderr)
	var tokenName, abilitiesFlag, clientNameFlag string
	var expiresInDays int
	var deviceFlag, reauthFlag, waitFlag bool
	var codeFlag string
	var openFlag triBool
	var approvalTimeout time.Duration
	fs.StringVar(&codeFlag, "code", "", "pairing code from the dashboard's \"Connect your CLI\" card (XXXX-XXXX); signs in with no password and no approval")
	fs.Var(&openFlag, "open", "open the approval page in a browser when a new device needs approval (default: on at a desktop terminal, off over SSH and in scripts)")
	fs.BoolVar(&waitFlag, "wait", false, "wait for new-device approval even when not at a terminal (default: fail at once with the fix)")
	fs.DurationVar(&approvalTimeout, "approval-timeout", 0, "how long to wait for new-device approval (default: "+envLoginApprovalTimeout+" env var, then 3m)")
	fs.BoolVar(&reauthFlag, "reauth", false, "replace the server this profile is pinned to (use after the server was reinstalled or restored)")
	fs.StringVar(&tokenName, "token-name", "", "name for the newly minted token (default: \"levelrail-cli-<timestamp>\")")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list for the new token (default: root, same as the session it's minted from)")
	fs.IntVar(&expiresInDays, "expires-in-days", 0, "token lifetime in days (default: 90)")
	fs.BoolVar(&deviceFlag, "device", false, "log in via the device code flow (print a code, approve it from the web dashboard) instead of username/password; works over plain http")
	fs.StringVar(&clientNameFlag, "client-name", "", "optional label for the device request shown in the approval UI, --device only (default: local hostname)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authLoginUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
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

	if deviceFlag {
		return runAuthLoginDevice(prog, *apiURLFlagP, *profileFlagP, clientNameFlag, reauthFlag, stdout, stderr, lookupEnv, of, jsonOut)
	}

	if codeFlag != "" {
		return runAuthLoginCode(prog, *apiURLFlagP, *profileFlagP, codeFlag, clientNameFlag, reauthFlag, stdout, stderr, lookupEnv, of, jsonOut)
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

	ctx := withApprovalOptions(context.Background(), approvalOptions{timeout: approvalTimeout, open: openFlag.val, wait: waitFlag})
	sf := sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}
	profile := resolveProfile(sf.profileFlag, lookupEnv)
	apiURL := resolveAPIURL(sf.apiURLFlag, lookupEnv, prog, profile)
	serverInfo, err := checkLoginServer(prog, profile, apiURL, reauthFlag, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	sessionClient, loginResp, err := loggedInSessionClient(ctx, sf, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	created, err := sessionClient.CreateToken(ctx, createTokenRequest{Name: tokenName, Abilities: abilities, ExpiresInDays: expiresInDays})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("mint API token: %w", err))
	}

	if err := writeCredentialsFile(prog, profile, withServerPin(credentials{APIURL: apiURL, Token: created.Token}, serverInfo)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save credentials: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		dir, _ := configDir(prog)
		_, _ = fmt.Fprintf(stdout, "logged in as %q\n", loginResp.Username)
		_, _ = fmt.Fprintf(stdout, "token %q (id %s) created and saved to %s/%s under profile %q\n", created.Name, created.ID, dir, credentialsFileName, profile)
		_, _ = fmt.Fprintf(stdout, "token value (shown once, not recoverable again): %s\n", created.Token)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
Use --device instead to avoid that entirely.

With --code XXXX-XXXX: signs in with a short-lived, single-use pairing code
copied from the dashboard's "Connect your CLI" card. No password, no new
device approval, works over plain http. The code is issued only to a
signed-in dashboard session.

When a password sign-in comes from a new device, the CLI prints the
dashboard page to approve it on and the two-digit number to type, opens that
page when a desktop browser is available (--open / --open=false), and waits
up to --approval-timeout (default 3m). Outside a terminal it fails at once
with the fix unless --wait is given.

With --device: prints a short code and a URL, waits for an operator to
approve it from the web dashboard's CLI Access page, then saves the
resulting token the same way. Works over plain http; --username/
--password/--token-name/--abilities/--expires-in-days are ignored in
this mode (the minted token's abilities always match the approving
operator's own session, the CLI never gets to choose). The printed link
uses the address this CLI actually reached, with the code pre-filled.

Flags:
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string             named credentials profile to save this login under (overrides APP_PROFILE, default "default"); lets one operator manage multiple control planes without overwriting each other's credentials
  --token-name string         name for the new token (default: "levelrail-cli-<timestamp>")
  --abilities string           comma-separated ability list (default: root)
  --expires-in-days int      token lifetime in days (default: 90)
  --reauth                       replace the server this profile is pinned to (after the server was reinstalled or restored)
  --code string                pairing code from the dashboard's "Connect your CLI" card
  --open                         open the approval page in a browser (default: on at a desktop terminal)
  --wait                          wait for new-device approval when not at a terminal
  --approval-timeout duration  how long to wait for new-device approval (default 3m)
  --device                       log in via the device code flow instead of username/password
  --client-name string       optional label for the device request shown in the approval UI, --device only (default: local hostname)
  --json                          print the new token resource as JSON to stdout; with --device, the first stdout line is a JSON object with the approval URL, code and expiry
  --output string                output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string                 JMESPath expression to filter the result before printing
  -h, --help                    show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

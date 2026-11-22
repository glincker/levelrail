package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Defaults to root because a session is always root, so a narrower default would silently weaken the token.
// Use --abilities for a deliberately narrower credential.
var defaultTokenAbility = []string{"root"}

// Plain http to a non-loopback host is refused before any credential is sent, unless --insecure-http or the env override is set.
func runAuthLogin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth login", "print the new token resource as JSON to stdout and nothing else", stderr)
	var tokenName, abilitiesFlag, clientNameFlag string
	var expiresInDays int
	var deviceFlag, reauthFlag, waitFlag, printToken, forceFlag, insecureHTTP, withToken bool
	var codeFlag string
	var openFlag triBool
	var approvalTimeout time.Duration
	fs.BoolVar(&insecureHTTP, "insecure-http", false, "allow sending credentials over plain http to a non-loopback host (default: refused; "+apiclient.EnvInsecureHTTP+"=1 does the same)")
	fs.StringVar(&codeFlag, "code", "", "pairing code from the dashboard's \"Connect your CLI\" card (XXXX-XXXX); signs in with no password and no approval")
	fs.Var(&openFlag, "open", "open the approval page in a browser when a new device needs approval (default: on at a desktop terminal, off over SSH and in scripts)")
	fs.BoolVar(&waitFlag, "wait", false, "wait for new-device approval even when not at a terminal (default: fail at once with the fix)")
	fs.DurationVar(&approvalTimeout, "approval-timeout", 0, "how long to wait for new-device approval (default: "+envLoginApprovalTimeout+" env var, then 3m)")
	fs.BoolVar(&withToken, "with-token", false, "read an existing API token from the first line of stdin and save it (no token on argv)")
	fs.BoolVar(&withToken, "token-stdin", false, "same as --with-token")
	fs.BoolVar(&reauthFlag, "reauth", false, "replace the server this profile is pinned to (use after the server was reinstalled or restored)")
	fs.BoolVar(&forceFlag, "force", false, "sign in again even when a valid saved login exists (default: reuse it and make no new session)")
	fs.StringVar(&tokenName, "token-name", "", "name for the newly minted token (default: \"levelrail-cli-<timestamp>\")")
	fs.StringVar(&abilitiesFlag, "abilities", "", "comma-separated ability list for the new token (default: root, same as the session it's minted from)")
	fs.IntVar(&expiresInDays, "expires-in-days", 0, "token lifetime in days (default: 90)")
	fs.BoolVar(&deviceFlag, "device", false, "log in via the device code flow (print a code, approve it from the web dashboard) instead of username/password; works over plain http")
	fs.StringVar(&clientNameFlag, "client-name", "", "optional label for the device request shown in the approval UI, --device only (default: local hostname)")
	fs.BoolVar(&printToken, "print-token", false, "print the new token value to stdout (default: only save it to the credentials file)")
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

	if insecureHTTP {
		apiclient.SetInsecureHTTP(true)
	}
	loginURL := resolveAPIURL(*apiURLFlagP, lookupEnv, prog, resolveProfile(*profileFlagP, lookupEnv))
	if err := apiclient.CheckCleartextHTTP(loginURL, insecureHTTP, lookupEnv); err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s", err.Error()))
	}

	// A password, device cookie or minted token only goes to an address the user saved or typed.
	if m, bad := savedLoginMismatch(*apiURLFlagP, lookupEnv, prog, resolveProfile(*profileFlagP, lookupEnv)); bad {
		return reportError(stdout, stderr, jsonOut, m)
	}

	if withToken {
		return runAuthLoginWithToken(prog, *apiURLFlagP, *profileFlagP, stdin, stdout, stderr, lookupEnv, of, jsonOut, reauthFlag)
	}

	if !forceFlag && !reauthFlag {
		if st, ok := reusableLogin(prog, *profileFlagP, *apiURLFlagP, lookupEnv); ok {
			return reportReused(st, stdout, stderr, of)
		}
	}

	if deviceFlag {
		return runAuthLoginDevice(prog, *apiURLFlagP, *profileFlagP, clientNameFlag, reauthFlag, printToken, stdout, stderr, lookupEnv, of, jsonOut)
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
	if err := requireHumanForLogin(prog, profile, apiURL, sf, stdin); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	serverInfo, err := checkLoginServer(prog, profile, apiURL, reauthFlag, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	var old replacedLogin
	var haveOld bool
	if forceFlag || reauthFlag {
		old, haveOld = savedLoginToReplace(prog, profile, apiURL, lookupEnv)
	}
	sessionClient, loginResp, err := loggedInSessionClient(ctx, sf, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	created, err := sessionClient.CreateToken(ctx, createTokenRequest{Name: tokenName, Abilities: abilities, ExpiresInDays: expiresInDays})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("mint API token: %w", err))
	}

	if err := writeCredentialsFile(prog, profile, withServerPin(credentials{APIURL: apiURL, Token: created.Token, TrustedDevice: sessionClient.trustedDevice(), TokenExpiresAt: expiryString(created.ExpiresAt)}, serverInfo)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save credentials: %w", err))
	}
	if haveOld {
		revokeReplaced(ctx, sessionClient, old, created.ID)
	}
	// The token carries on its own; ending the login session keeps the server's session list to real browsers.
	_ = sessionClient.Logout(ctx)

	// The token is already saved; keeping it off stdout stops it leaking into logs and scrollback.
	shown := created
	if !printToken {
		shown.Token = ""
	}
	if err := renderResult(stdout, of.Format, of.Query, shown, func() {
		dir, _ := configDir(prog)
		_, _ = fmt.Fprintf(stdout, "logged in as %q\n", loginResp.Username)
		_, _ = fmt.Fprintf(stdout, "token %q (id %s) created and saved to %s/%s under profile %q\n", created.Name, created.ID, dir, credentialsFileName, profile)
		if printToken {
			_, _ = fmt.Fprintf(stdout, "token value (shown once, not recoverable again): %s\n", created.Token)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// reportReused tells the caller an existing login was kept, so nothing new was created on the server.
func reportReused(st sessionStatus, stdout, stderr io.Writer, of outputFlags) int {
	if err := renderResult(stdout, of.Format, of.Query, st, func() {
		_, _ = fmt.Fprintf(stdout, "already signed in as %q on profile %q (%s), reusing the saved login: no new session, no approval needed. Pass --force to sign in again.\n", st.Identity, st.Profile, st.APIURL)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// requireHumanForLogin refuses to prompt when nobody is at the terminal, naming the command a person runs instead.
func requireHumanForLogin(prog, profile, apiURL string, sf sessionFlags, stdin io.Reader) error {
	if stdin != io.Reader(os.Stdin) || stdinIsTerminal() || (sf.username != "" && sf.password != "") {
		return nil
	}
	return newValidationError("no valid credential for profile %q and this is not a terminal, so no login can be started here. A person must run: %s. Or paste a one-time code from the dashboard's \"Connect your CLI\" card: %s login --api-url %s --profile %s --code XXXX-XXXX. State as JSON: %s whoami --json",
		profile, loginFix(prog, profile, apiURL), prog, apiURL, profile, prog)
}

// expiryString is the RFC3339 form credentials store, empty when the token never expires.
func expiryString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
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

If this profile already holds a login the server accepts, nothing happens:
the saved login is reused (no new session, no new-device approval). --force
signs in again. With no terminal and no valid login it fails at once and
names the command a person must run.

Authenticates with a username and password (prompted interactively,
without echo for the password, when not given as flags), then mints a
real API token from that session and saves it to this CLI's own
credentials file. Every later command's --token/%[2]s/credentials-file
resolution then finds it automatically.

Plain-http targets work on loopback (127.0.0.1, localhost, ::1). For any
other host use an https --api-url; plain http to another host is refused
unless --insecure-http (or the APP_INSECURE_HTTP env var) is set. The new token is saved to the credentials file and is not
printed unless you pass --print-token.

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
  --print-token                print the new token value to stdout (default: saved only)
  --insecure-http            allow plain http to a non-loopback host (default: refused)
  --force                         sign in again even when a valid saved login exists (default: reuse it, no new session)
  --reauth                       replace the server this profile is pinned to (after the server was reinstalled or restored)
  --with-token                read an existing API token from the first line of stdin and save it after the server accepts it (alias --token-stdin); example: %[1]s login --with-token < token.txt
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
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

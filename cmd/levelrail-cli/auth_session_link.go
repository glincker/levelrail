package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// runAuthSessionLink mints a session link via POST /api/v1/auth/session-links. The server requires AbilityRoot.
func runAuthSessionLink(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth session-link", "print the new session link as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authSessionLinkUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	created, err := client.MintSessionLink(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("mint session link: %w", err))
	}

	created.URL = sessionLinkForClient(created.URL, client.BaseURL())

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintln(stdout, created.URL)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// sessionLinkForClient makes the link point at the host the CLI talks to.
// A server without a primary domain mints a localhost link, which is wrong
// for anyone using a remote profile.
func sessionLinkForClient(link, base string) string {
	if strings.HasPrefix(link, "/") {
		return strings.TrimRight(base, "/") + link
	}
	l, err := url.Parse(link)
	b, berr := url.Parse(base)
	if err != nil || berr != nil || b.Host == "" || !isLoopbackHost(l.Hostname()) || isLoopbackHost(b.Hostname()) {
		return link
	}
	l.Scheme, l.Host = b.Scheme, b.Host
	return l.String()
}

func isLoopbackHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func authSessionLinkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth session-link [flags]

Mints a short-lived (about 2 minutes), single-use login link: opening it
in a browser signs in as whoever minted it, no password needed. Meant
for browser automation driving the dashboard from an already-authenticated
context. Requires a token with the root ability.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the new session link as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

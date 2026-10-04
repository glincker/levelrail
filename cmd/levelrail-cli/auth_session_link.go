package main

import (
	"context"
	"fmt"
	"io"
)

// runAuthSessionLink implements "auth session-link": POST
// /api/v1/auth/session-links via client.go's own Client.MintSessionLink,
// the same resolved-token auth every other command in this CLI uses.
// Requires the token to hold AbilityRoot, enforced server-side. Prints
// the ready-to-open URL and nothing else: opening it signs the browser
// in without any further command.
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

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintln(stdout, created.URL)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
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

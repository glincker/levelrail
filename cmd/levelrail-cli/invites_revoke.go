package main

import (
	"context"
	"fmt"
	"io"
)

// runInvitesRevoke implements "invites revoke <id>": DELETE
// /api/v1/invites/{id}. Refused (400) server-side for an invite already
// accepted or already revoked (handleRevokeInvite's own doc comment),
// surfaced here as-is.
func runInvitesRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "invites revoke", "print {\"revoked\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, invitesRevokeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	id, ok := requireOneArg(fs, stderr, prog, "invites revoke", "invite id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.RevokeInvite(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke invite %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"revoked": true}, func() {
		_, _ = fmt.Fprintf(stdout, "invite %q revoked\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func invitesRevokeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s invites revoke <id> [flags]

Revokes a pending invite. Refused (400) if it was already accepted or
already revoked.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {"revoked": true} as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

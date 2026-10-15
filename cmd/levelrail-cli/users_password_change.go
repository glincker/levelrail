package main

import (
	"context"
	"fmt"
	"io"
)

// runUsersRequirePasswordChange implements "users require-password-change <id> [--off]".
// Requiring it ends the user's sessions; sign-in is refused until a password reset.
func runUsersRequirePasswordChange(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "users require-password-change", "print the updated user as JSON to stdout on success and nothing else", stderr)
	off := fs.Bool("off", false, "clear the requirement instead of setting it")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usersRequirePasswordChangeUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "users require-password-change", "user id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	u, err := client.SetPasswordChangeRequired(context.Background(), id, !*off)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set password change for %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, u, func() {
		if u.MustChangePassword {
			_, _ = fmt.Fprintf(stdout, "user %q must change their password before signing in again\n", u.Email)
			return
		}
		_, _ = fmt.Fprintf(stdout, "user %q no longer has to change their password\n", u.Email)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func usersRequirePasswordChangeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s users require-password-change <id> [--off] [flags]

Requires the user to choose a new password. Their sessions end at once, and
every sign-in is refused until they complete a password reset (forgot
password on the sign-in page). --off clears the requirement. Refused for
your own account and for accounts without a password.

Flags:
  --off                    clear the requirement instead of setting it
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated user as JSON to stdout on success, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

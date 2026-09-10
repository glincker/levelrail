package main

import (
	"context"
	"fmt"
	"io"
)

// runAuthTwoFactorDisable implements "auth 2fa disable": POST
// /api/v1/auth/2fa/disable, re-verifying the second factor itself (a
// live code or a recovery code) rather than the account password, same
// reasoning handleDisableTwoFactor's own doc comment gives.
func runAuthTwoFactorDisable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth 2fa disable", "print a JSON result object to stdout and nothing else", stderr)
	var code, recoveryCode string
	fs.StringVar(&code, "code", "", "current 6-digit code from the authenticator app")
	fs.StringVar(&recoveryCode, "recovery-code", "", "a single-use recovery code, alternative to --code")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authTwoFactorDisableUsage(prog)) }

	jsonOut, of, exitCode, ok := parseSessionFlags(fs, args, jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}

	if code == "" && recoveryCode == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("either --code or --recovery-code is required"))
	}

	ctx := context.Background()
	sessionClient, exitCode, ok := buildSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stdout, stderr, jsonOut)
	if !ok {
		return exitCode
	}

	if err := sessionClient.DisableTwoFactor(ctx, code, recoveryCode); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable two-factor authentication: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"status": "disabled"}, func() {
		_, _ = fmt.Fprintln(stdout, "two-factor authentication disabled")
	})
}

func authTwoFactorDisableUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth 2fa disable --code CODE|--recovery-code CODE [flags]

Turns two-factor authentication off, requiring proof of the second
factor itself (a live code or a recovery code), never just the account
password.

Flags:
  --code string                 current 6-digit code from the authenticator app
  --recovery-code string   a single-use recovery code, alternative to --code
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print a JSON result object to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

package main

import (
	"context"
	"fmt"
	"io"
)

// runAuthTwoFactorEnable implements "auth 2fa enable --code CODE": POST
// /api/v1/auth/2fa/confirm, the second half of setup. On success the
// server returns a fresh set of recovery codes in plaintext, shown here
// exactly once, never recoverable from the server again afterward.
func runAuthTwoFactorEnable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth 2fa enable", "print the recovery codes as a JSON array to stdout and nothing else", stderr)
	var code string
	fs.StringVar(&code, "code", "", "current 6-digit code from the authenticator app (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authTwoFactorEnableUsage(prog)) }

	jsonOut, of, exitCode, ok := parseSessionFlags(fs, args, jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}

	if code == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--code is required"))
	}

	ctx := context.Background()
	sessionClient, exitCode, ok := buildSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stdout, stderr, jsonOut)
	if !ok {
		return exitCode
	}

	recovery, err := sessionClient.EnableTwoFactor(ctx, code)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("enable two-factor authentication: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, recovery, func() { printRecoveryCodes(stdout, recovery) })
}

func printRecoveryCodes(out io.Writer, recovery twoFactorRecoveryCodesResponse) {
	_, _ = fmt.Fprintln(out, "two-factor authentication enabled")
	_, _ = fmt.Fprintln(out, "recovery codes (shown once, not recoverable again), store them somewhere safe:")
	for _, code := range recovery.RecoveryCodes {
		_, _ = fmt.Fprintln(out, code)
	}
}

func authTwoFactorEnableUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth 2fa enable --code CODE [flags]

Confirms enrollment by validating a live code from the authenticator app
set up with "%[1]s auth 2fa setup". Prints a fresh set of recovery codes
once, never recoverable from the server again afterward.

Flags:
  --code string                 current 6-digit code from the authenticator app (required)
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the recovery codes as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

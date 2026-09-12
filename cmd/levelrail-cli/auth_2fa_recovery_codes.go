package main

import (
	"fmt"
	"io"
)

// runAuthTwoFactorRecoveryCodes implements "auth 2fa recovery-codes
// --code CODE": POST /api/v1/auth/2fa/recovery-codes/regenerate,
// replacing the whole set and invalidating every previously issued code.
// Gated on a live TOTP code specifically, never a recovery code, so
// regenerating never consumes one of the codes it's about to throw away.
// Use "auth 2fa status" to see how many of the current set remain
// unused; there is no endpoint to view the current codes' plaintext
// again once shown.
func runAuthTwoFactorRecoveryCodes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth 2fa recovery-codes", "print the new recovery codes as a JSON array to stdout and nothing else", stderr)
	var code string
	fs.StringVar(&code, "code", "", "current 6-digit code from the authenticator app (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authTwoFactorRecoveryCodesUsage(prog)) }

	flags := sessionFlagPtrs{usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}
	ctx, sessionClient, jsonOut, of, exitCode, ok := requireCodeAndSessionClient(fs, args, &code, flags, prog, lookupEnv, stdin, stdout, stderr)
	if !ok {
		return exitCode
	}

	recovery, err := sessionClient.RegenerateRecoveryCodes(ctx, code)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("regenerate recovery codes: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, recovery, func() {
		_, _ = fmt.Fprintln(stdout, "recovery codes regenerated, the old set is now invalid")
		_, _ = fmt.Fprintln(stdout, "recovery codes (shown once, not recoverable again), store them somewhere safe:")
		for _, c := range recovery.RecoveryCodes {
			_, _ = fmt.Fprintln(stdout, c)
		}
	})
}

func authTwoFactorRecoveryCodesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth 2fa recovery-codes --code CODE [flags]

Regenerates recovery codes, invalidating every previously issued code.
Requires a live authenticator code, never a recovery code, so this can't
consume one of the codes it's about to throw away. Prints the new set
once, never recoverable from the server again afterward; "%[1]s auth 2fa
status" shows only how many remain, not their plaintext.

Flags:
  --code string                 current 6-digit code from the authenticator app (required)
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the new recovery codes as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

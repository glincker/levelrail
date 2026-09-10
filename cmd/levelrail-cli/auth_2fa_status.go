package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAuthTwoFactorStatus implements "auth 2fa status": GET
// /api/v1/auth/2fa, whether the logged-in account has TOTP enabled and,
// if so, how many recovery codes remain unused.
func runAuthTwoFactorStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth 2fa status", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authTwoFactorStatusUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
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

	ctx := context.Background()
	sessionClient, _, err := loggedInSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	status, err := sessionClient.TwoFactorStatus(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get two-factor status: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, status, func() { printTwoFactorStatus(stdout, status) })
}

func printTwoFactorStatus(out io.Writer, status twoFactorStatusResponse) {
	if !status.Enabled {
		_, _ = fmt.Fprintln(out, "two-factor authentication: disabled")
		return
	}
	_, _ = fmt.Fprintln(out, "two-factor authentication: enabled")
	_, _ = fmt.Fprintf(out, "recovery codes remaining: %d\n", status.RecoveryCodesRemaining)
}

func authTwoFactorStatusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth 2fa status [flags]

Shows whether the logged-in account has two-factor authentication
enabled and, if so, how many recovery codes are left unused.

Flags:
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the status as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

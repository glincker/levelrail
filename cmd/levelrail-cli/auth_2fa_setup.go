package main

import (
	"context"
	"fmt"
	"io"
)

// runAuthTwoFactorSetup implements "auth 2fa setup": POST
// /api/v1/auth/2fa/setup, minting a fresh unconfirmed TOTP secret. Run
// "auth 2fa enable --code CODE" next with a live code from an
// authenticator app seeded from the printed secret or provisioning URI
// to finish enrollment.
func runAuthTwoFactorSetup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, usernameP, passwordP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := sessionFlagSet(prog, "auth 2fa setup", "print the secret and provisioning URI as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authTwoFactorSetupUsage(prog)) }

	jsonOut, of, exitCode, ok := parseSessionFlags(fs, args, jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}

	ctx := context.Background()
	sessionClient, exitCode, ok := buildSessionClient(ctx, sessionFlags{*usernameP, *passwordP, *apiURLFlagP, *profileFlagP}, prog, lookupEnv, stdin, stdout, stderr, jsonOut)
	if !ok {
		return exitCode
	}

	setup, err := sessionClient.SetupTwoFactor(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set up two-factor authentication: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, setup, func() {
		_, _ = fmt.Fprintf(stdout, "secret: %s\n", setup.Secret)
		_, _ = fmt.Fprintf(stdout, "provisioning URI: %s\n", setup.ProvisioningURI)
		_, _ = fmt.Fprintln(stdout, "add this to an authenticator app, then run \"auth 2fa enable --code CODE\" with a live code to finish enrollment")
	})
}

func authTwoFactorSetupUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth 2fa setup [flags]

Starts enrollment: mints a fresh TOTP secret and provisioning URI (add
either to an authenticator app), unconfirmed until "%[1]s auth 2fa enable
--code CODE" validates a live code against it. Calling this again before
enable just overwrites the pending secret.

Flags:
  --username string          admin username (prompted if omitted)
  --password string          admin password (prompted without echo if omitted)
  --api-url string             control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string             named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                          print the secret and provisioning URI as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIURL, defaultAPIURL)
}

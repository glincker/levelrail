package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAuthTwoFactor dispatches "auth 2fa <verb> [flags]" to one of
// status/setup/enable/disable/recovery-codes: internal/api/twofactor.go's
// account-security routes, session-only like every route under "auth"
// (see auth.go's own doc comment), never bearer-token authenticated.
func runAuthTwoFactor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authTwoFactorUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authTwoFactorUsage(prog))
		return exitOK
	case "status":
		return runAuthTwoFactorStatus(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	case "setup":
		return runAuthTwoFactorSetup(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	case "enable":
		return runAuthTwoFactorEnable(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	case "disable":
		return runAuthTwoFactorDisable(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	case "recovery-codes":
		return runAuthTwoFactorRecoveryCodes(prog, args[1:], stdout, stderr, lookupEnv, stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth 2fa subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authTwoFactorUsage(prog))
		return exitUsage
	}
}

// requireCodeAndSessionClient is "auth 2fa enable"/"auth 2fa
// recovery-codes"'s shared prelude: parse flags, require --code (both
// subcommands need a live authenticator code and nothing else), then log
// in. ok is false once it has already returned the caller's exitCode;
// the caller should return immediately in that case.
func requireCodeAndSessionClient(fs *flag.FlagSet, args []string, codeP *string, flags sessionFlagPtrs, prog string, lookupEnv func(string) (string, bool), stdin io.Reader, stdout, stderr io.Writer) (ctx context.Context, client *authSessionClient, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	jsonOut, of, exitCode, ok = parseSessionFlags(fs, args, flags.jsonOut, flags.output, flags.query, prog, stderr)
	if !ok {
		return nil, nil, false, outputFlags{}, exitCode, false
	}
	if *codeP == "" {
		return nil, nil, false, outputFlags{}, reportError(stdout, stderr, jsonOut, newValidationError("--code is required")), false
	}
	ctx = context.Background()
	client, exitCode, ok = buildSessionClient(ctx, sessionFlags{*flags.username, *flags.password, *flags.apiURL, *flags.profile}, prog, lookupEnv, stdin, stdout, stderr, jsonOut)
	if !ok {
		return nil, nil, false, outputFlags{}, exitCode, false
	}
	return ctx, client, jsonOut, of, 0, true
}

func authTwoFactorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth 2fa status [flags]                                          show whether two-factor auth is enabled
  %[1]s auth 2fa setup [flags]                                             start enrollment, returns a secret and provisioning URI
  %[1]s auth 2fa enable --code CODE [flags]                          confirm enrollment, returns recovery codes once
  %[1]s auth 2fa disable --code CODE|--recovery-code CODE [flags]   turn two-factor auth off
  %[1]s auth 2fa recovery-codes --code CODE [flags]                regenerate recovery codes, invalidating the old set

Every subcommand here requires a live session: --username/--password
(prompted if omitted), same as "%[1]s tokens" and "%[1]s auth login", never
this CLI's own persisted bearer token.

Run "%[1]s auth 2fa <subcommand> -h" for a subcommand's own flags.
`, prog)
}

package main

import (
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

package main

import (
	"fmt"
	"io"
	"os"
)

// Own top-level command because it needs session-cookie routes, unlike the bearer-token commands.
func runAuth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authUsage(prog))
		return exitOK
	case "login":
		return runAuthLogin(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "whoami":
		return runAuthWhoami(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runAuthStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "trust-server":
		return runAuthTrustServer(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "2fa":
		return runAuthTwoFactor(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	case "session-link":
		return runAuthSessionLink(prog, args[1:], stdout, stderr, lookupEnv)
	case "code":
		return runAuthCode(prog, args[1:], stdout, stderr, lookupEnv)
	case "approve":
		return runAuthDecide(prog, args[1:], true, stdout, stderr, lookupEnv)
	case "deny":
		return runAuthDecide(prog, args[1:], false, stdout, stderr, lookupEnv)
	case "devices":
		return runAuthDevices(prog, args[1:], stdout, stderr, lookupEnv)
	case "code-login":
		return runAuthCodeLogin(prog, args[1:], stdout, stderr, lookupEnv)
	case "activity":
		return runAuthActivity(prog, args[1:], stdout, stderr, lookupEnv)
	case "preferences":
		return runAuthPreferences(prog, args[1:], stdout, stderr, lookupEnv)
	case "export":
		return runAuthExport(prog, args[1:], stdout, stderr, lookupEnv)
	case "unlink":
		return runAuthUnlink(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete-account":
		return runAuthDeleteAccount(prog, args[1:], stdout, stderr, lookupEnv, os.Stdin)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authUsage(prog))
		return exitUsage
	}
}

func authUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth login [flags]           authenticate and persist a new API token
  %[1]s auth whoami [flags]          show who the current token authenticates as
  %[1]s auth status [flags]          show the server this profile is pinned to and check it
  %[1]s auth trust-server [flags]    pin the server at the profile's URL (after confirming its fingerprint)
  %[1]s auth 2fa <verb>             manage this account's own two-factor authentication
  %[1]s auth session-link [flags]    mint a short-lived, single-use login link
  %[1]s auth code [flags]            show sign-in codes and new-browser sign-ins waiting for you
  %[1]s auth approve <id>            approve a new-browser sign-in
  %[1]s auth deny <id>               deny a new-browser sign-in
  %[1]s auth devices [revoke <id>]   list or revoke browsers trusted for password sign-in
  %[1]s auth code-login [flags]      show or change the auth.code_login setting
  %[1]s auth activity [flags]        show your own account activity feed
  %[1]s auth preferences get|set     your timezone, language, theme, landing page and avatar
  %[1]s auth export [flags]          download a zip of your own account data
  %[1]s auth unlink <provider>       remove a linked sign-in provider
  %[1]s auth delete-account [flags]  permanently delete your own account

Run "%[1]s auth <subcommand> -h" for a subcommand's own flags.
`, prog)
}

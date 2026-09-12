package main

import (
	"fmt"
	"io"
)

// runSettings dispatches "settings <resource> <verb> [flags]" to one of
// oauth/email/ingress, the three instance-wide settings resources that
// had a dashboard page and an API route but no CLI surface at all: a
// headless, browser-less first-run setup (install.sh plus this CLI) had
// no way to touch OAuth sign-in, SMTP/SES, or ingress/ACME config before
// this. Each resource keeps its own file (settings_oauth.go,
// settings_email.go, settings_ingress.go), the same "one file per
// sub-resource" shape apps_git_source.go/apps_log_drain.go already
// establish for a nested dispatch group.
func runSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsUsage(prog))
		return exitOK
	case "oauth":
		return runSettingsOAuth(prog, args[1:], stdout, stderr, lookupEnv)
	case "email":
		return runSettingsEmail(prog, args[1:], stdout, stderr, lookupEnv)
	case "ingress":
		return runSettingsIngress(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsUsage(prog))
		return exitUsage
	}
}

func settingsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings oauth list [flags]                     show every OAuth sign-in provider's current settings
  %[1]s settings oauth set <provider> [flags]           enable/configure/disable one OAuth sign-in provider
  %[1]s settings email get [flags]                        show the current outbound email (SMTP/SES) settings
  %[1]s settings email set [flags]                        configure outbound email
  %[1]s settings ingress get [flags]                      show the current ingress/ACME settings
  %[1]s settings ingress set [flags]                      configure the primary domain and ACME (Let's Encrypt) settings

Instance-wide configuration, gated at AbilityRoot server-side on every
write. <provider> for "settings oauth set" is one of "google", "github",
or "oidc".

Run "%[1]s settings <resource> <subcommand> -h" for a subcommand's own flags.
`, prog)
}

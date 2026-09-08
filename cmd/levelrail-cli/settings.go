package main

import (
	"fmt"
	"io"
)

// runSettings dispatches "settings <resource> <verb> [flags]" to one of
// email/oauth/ingress: the global, control-plane-wide configuration
// areas internal/api/routes.go and routes_platform.go's own
// "settings/*" routes expose, distinct from "domains cloudflare-dns"
// (a per-domain DNS-01 credential) and "cloudflare-tunnel" (the
// cloudflared connector), both already top-level commands of their own.
func runSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsUsage(prog))
		return exitOK
	case "email":
		return runSettingsEmail(prog, args[1:], stdout, stderr, lookupEnv)
	case "oauth":
		return runSettingsOAuth(prog, args[1:], stdout, stderr, lookupEnv)
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
  %[1]s settings email get|set [flags]     outbound email (SMTP or SES) for password resets and invites
  %[1]s settings oauth list|set [flags]    OAuth sign-in providers (google, github, oidc)
  %[1]s settings ingress get|set|check [flags]   embedded Caddy ingress: primary domain and ACME

Run "%[1]s settings <subcommand> -h" for a subcommand's own flags.
`, prog)
}

package main

import (
	"fmt"
	"io"
)

// Covers oauth, email, and ingress, which a headless first-run setup otherwise cannot reach.
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
	case "edge":
		return runSettingsEdge(prog, args[1:], stdout, stderr, lookupEnv)
	case "dashboard-url":
		return runSettingsDashboardURL(prog, args[1:], stdout, stderr, lookupEnv)
	case "ai-assistant":
		return runSettingsAIAssistant(prog, args[1:], stdout, stderr, lookupEnv)
	case "updates":
		return runSettingsUpdates(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy-freeze":
		return runSettingsDeployFreeze(prog, args[1:], stdout, stderr, lookupEnv)
	case "domain-automation":
		return runSettingsDomainAutomation(prog, args[1:], stdout, stderr, lookupEnv)
	case "beta-features":
		return runSettingsBetaFeatures(prog, args[1:], stdout, stderr, lookupEnv)
	case "profile":
		return runSettingsProfile(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsUsage(prog))
		return exitUsage
	}
}

func settingsUsage(prog string) string {
	return filterExperimentalUsage(fmt.Sprintf(`Usage:
  %[1]s settings oauth list [flags]                     show every OAuth sign-in provider's current settings
  %[1]s settings oauth set <provider> [flags]           enable/configure/disable one OAuth sign-in provider
  %[1]s settings email get [flags]                        show the current outbound email (SMTP/SES) settings
  %[1]s settings email set [flags]                        configure outbound email
  %[1]s settings email branding get|set|preview [flags]   logo, accent color and footer for rich email
  %[1]s settings ingress get [flags]                      show the current ingress/ACME settings
  %[1]s settings ingress set [flags]                      configure the primary domain and ACME (Let's Encrypt) settings
  %[1]s settings edge get|set|fix [flags]                 request limits and trusted proxies (fix detects the proxy in front)
  %[1]s settings dashboard-url get [flags]                show the public URL operators use to reach the dashboard
  %[1]s settings dashboard-url set --url URL [flags]      set it (https refuses plain-HTTP sign-in afterwards)
  %[1]s settings ai-assistant get [flags]                 show the current BYOK AI assistant settings
  %[1]s settings ai-assistant set --model NAME --api-key KEY [flags]   configure the BYOK AI assistant
  %[1]s settings ai-assistant clear [flags]               disable the AI assistant and forget the stored key
  %[1]s settings updates get [flags]                      show the current release channel and auto-update-check setting
  %[1]s settings updates set --channel CHANNEL [--auto-update] [flags]   configure the release channel and auto-update checking
  %[1]s settings domain-automation get|set [flags]        the add-a-domain-and-go-live policy
  %[1]s settings beta-features list|set <feature> on|off   gated features, switch one (saved at once)
  %[1]s settings profile get|set minimal|standard|suite   the server profile: which optional subsystems start
  %[1]s settings deploy-freeze show [flags]             show the fleet-wide deploy freeze windows
  %[1]s settings deploy-freeze set --cron EXPR --duration D [flags]   replace them with one window
  %[1]s settings deploy-freeze clear [flags]              clear the fleet-wide freeze windows

Instance-wide configuration, gated at AbilityRoot server-side on every
write. <provider> for "settings oauth set" is one of "google", "github",
or "oidc".

Run "%[1]s settings <resource> <subcommand> -h" for a subcommand's own flags.
`, prog))
}

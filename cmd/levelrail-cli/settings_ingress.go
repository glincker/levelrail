package main

import (
	"context"
	"fmt"
	"io"
)

// runSettingsIngress dispatches "settings ingress <verb> [flags]" to one
// of get/set, the singleton primary-domain/ACME configuration
// (internal/api/ingress_settings.go). This is the CLI counterpart of the
// dashboard's own Domains page ingress card; a domain check
// (GET /api/v1/settings/ingress/check) stays dashboard-only for now,
// since it exists to render a visual DNS status, not to script setup.
func runSettingsIngress(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsIngressUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsIngressUsage(prog))
		return exitOK
	case "get":
		return runSettingsIngressGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsIngressSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ingress subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsIngressUsage(prog))
		return exitUsage
	}
}

func settingsIngressUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ingress get [flags]
  %[1]s settings ingress set [--primary-domain DOMAIN] [--acme-enabled] [--acme-email EMAIL] [flags]

Configures the platform-wide primary domain and ACME (Let's Encrypt)
certificate automation. --acme-email is required whenever --acme-enabled
is set.

Run "%[1]s settings ingress <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsIngressGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress get", "print the ingress settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress get [flags]\n\nShows the current primary domain and ACME settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetIngressSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ingress settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printIngressSettingsHuman(stdout, settings) })
}

func runSettingsIngressSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress set", "print the updated ingress settings as JSON to stdout and nothing else", stderr)
	var primaryDomain, acmeEmail, acmeDirectoryURL string
	var acmeEnabled bool
	fs.StringVar(&primaryDomain, "primary-domain", "", "hostname the dashboard itself is reachable at")
	fs.BoolVar(&acmeEnabled, "acme-enabled", false, "enable automatic TLS certificate issuance/renewal")
	fs.StringVar(&acmeEmail, "acme-email", "", "ACME account contact address (required when --acme-enabled is set)")
	fs.StringVar(&acmeDirectoryURL, "acme-directory-url", "", "ACME directory URL override (empty uses Caddy's own default, Let's Encrypt production)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress set [flags]\n\nConfigures the primary domain and ACME settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.UpdateIngressSettings(context.Background(), ingressSettingsResource{
		PrimaryDomain:    primaryDomain,
		ACMEEnabled:      acmeEnabled,
		ACMEEmail:        acmeEmail,
		ACMEDirectoryURL: acmeDirectoryURL,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ingress settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printIngressSettingsHuman(stdout, settings) })
}

func printIngressSettingsHuman(out io.Writer, s ingressSettingsResource) {
	_, _ = fmt.Fprintf(out, "primary_domain:     %s\n", s.PrimaryDomain)
	_, _ = fmt.Fprintf(out, "acme_enabled:       %v\n", s.ACMEEnabled)
	_, _ = fmt.Fprintf(out, "acme_email:         %s\n", s.ACMEEmail)
	_, _ = fmt.Fprintf(out, "acme_directory_url: %s\n", s.ACMEDirectoryURL)
}

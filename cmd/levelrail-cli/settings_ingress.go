package main

import (
	"context"
	"fmt"
	"io"
)

const settingsIngressJSONUsage = "print the settings as JSON to stdout and nothing else"

// runSettingsIngress dispatches "settings ingress <verb> [flags]" to one
// of get/set/check: get/set the embedded Caddy ingress's primary domain
// and ACME configuration, check runs the same DNS-pointed-here
// verification DomainEditor already offers per app
// (GET /api/v1/settings/ingress/check), against the platform's own
// primary domain instead.
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
	case "check":
		return runSettingsIngressCheck(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ingress subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsIngressUsage(prog))
		return exitUsage
	}
}

func settingsIngressUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ingress get [flags]                                    show the current settings
  %[1]s settings ingress set --primary-domain example.com [flags]        configure the embedded ingress
  %[1]s settings ingress check [flags]                                  verify the primary domain's DNS points here

Run "%[1]s settings ingress <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsIngressGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress get", settingsIngressJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress get [flags]\n\nShows the current ingress settings.\n\nFlags:\n", prog)
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

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printIngressSettingsHuman(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSettingsIngressSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress set", settingsIngressJSONUsage, stderr)
	var primaryDomainFlag, acmeEmailFlag, acmeDirectoryURLFlag string
	var acmeEnabledFlag bool
	fs.StringVar(&primaryDomainFlag, "primary-domain", "", "domain this control plane's own dashboard is reachable on")
	fs.BoolVar(&acmeEnabledFlag, "acme-enabled", false, "automatically obtain and renew TLS certificates via ACME")
	fs.StringVar(&acmeEmailFlag, "acme-email", "", "ACME account contact email (required when --acme-enabled)")
	fs.StringVar(&acmeDirectoryURLFlag, "acme-directory-url", "", "ACME directory URL (empty uses Caddy's own default)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress set [flags]\n\nConfigures the embedded Caddy ingress.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	req := ingressSettingsResource{
		PrimaryDomain:    primaryDomainFlag,
		ACMEEnabled:      acmeEnabledFlag,
		ACMEEmail:        acmeEmailFlag,
		ACMEDirectoryURL: acmeDirectoryURLFlag,
	}
	settings, err := client.SetIngressSettings(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ingress settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printIngressSettingsHuman(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSettingsIngressCheck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress check", settingsIngressJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress check [flags]\n\nVerifies the configured primary domain's DNS points at this control plane.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	result, err := client.CheckIngressDomain(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check ingress domain: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, result, func() { printIngressDomainCheckHuman(stdout, result) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printIngressSettingsHuman(out io.Writer, s ingressSettingsResource) {
	_, _ = fmt.Fprintf(out, "primary_domain:     %s\n", s.PrimaryDomain)
	_, _ = fmt.Fprintf(out, "acme_enabled:       %v\n", s.ACMEEnabled)
	_, _ = fmt.Fprintf(out, "acme_email:         %s\n", s.ACMEEmail)
	_, _ = fmt.Fprintf(out, "acme_directory_url: %s\n", s.ACMEDirectoryURL)
}

func printIngressDomainCheckHuman(out io.Writer, r ingressDomainCheckResource) {
	_, _ = fmt.Fprintf(out, "configured: %v\n", r.Configured)
	if !r.Configured {
		return
	}
	_, _ = fmt.Fprintf(out, "domain:        %s\n", r.Domain)
	_, _ = fmt.Fprintf(out, "expected_host: %s\n", r.ExpectedHost)
	_, _ = fmt.Fprintf(out, "resolved:      %v\n", r.Resolved)
	_, _ = fmt.Fprintf(out, "status:        %s\n", r.Status)
}

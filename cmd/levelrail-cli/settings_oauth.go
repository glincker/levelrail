package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

const settingsOAuthJSONUsage = "print the settings as JSON to stdout and nothing else"

// runSettingsOAuth dispatches "settings oauth <verb> [flags]" to one of
// list/set. "list" rather than "get": GET /api/v1/settings/oauth always
// returns every provider (google, github, oidc) in one array, the same
// list-shaped GET this package's own "registry-credentials list" and
// "backup-targets list" already use, not a single resource "get" would
// imply. "set" takes the provider as a positional argument, the same
// shape "registry-credentials update <id>" already uses for a
// per-resource PUT.
func runSettingsOAuth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsOAuthUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsOAuthUsage(prog))
		return exitOK
	case "list":
		return runSettingsOAuthList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsOAuthSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings oauth subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsOAuthUsage(prog))
		return exitUsage
	}
}

func settingsOAuthUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings oauth list [flags]                                     show every provider's settings
  %[1]s settings oauth set <provider> --enabled --client-id ID [flags]   configure a provider (google, github, or oidc)

Configures OAuth sign-in providers. The client secret is never returned
by "list": only whether one is currently stored.

Run "%[1]s settings oauth <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsOAuthList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings oauth list", settingsOAuthJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings oauth list [flags]\n\nShows every OAuth provider's settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.ListOAuthSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list oauth settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printOAuthSettingsTable(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runSettingsOAuthSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings oauth set", settingsOAuthJSONUsage, stderr)
	var enabledFlag bool
	var clientIDFlag, clientSecretFlag, allowedEmailDomainFlag, issuerURLFlag, displayNameFlag string
	fs.BoolVar(&enabledFlag, "enabled", true, "allow sign-in through this provider (pass --enabled=false to disable)")
	fs.StringVar(&clientIDFlag, "client-id", "", "OAuth client id (required to enable this provider)")
	fs.StringVar(&clientSecretFlag, "client-secret", "", "OAuth client secret (required the first time this provider is enabled; omit to keep the currently stored secret)")
	fs.StringVar(&allowedEmailDomainFlag, "allowed-email-domain", "", "restrict sign-in to email addresses on this domain (empty allows any)")
	fs.StringVar(&issuerURLFlag, "issuer-url", "", "OIDC issuer URL (required to enable the oidc provider)")
	fs.StringVar(&displayNameFlag, "display-name", "", "label shown on the sign-in button (oidc only)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings oauth set <provider> [flags]\n\nConfigures one OAuth provider: google, github, or oidc.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	provider, ok := requireOneArg(fs, stderr, prog, "settings oauth set", "provider")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	req := updateOAuthProviderSettingsRequest{
		Enabled:            enabledFlag,
		ClientID:           clientIDFlag,
		ClientSecret:       clientSecretFlag,
		AllowedEmailDomain: allowedEmailDomainFlag,
		IssuerURL:          issuerURLFlag,
		DisplayName:        displayNameFlag,
	}
	settings, err := client.SetOAuthProviderSettings(context.Background(), provider, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set oauth settings for %q: %w", provider, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printOAuthSettingsTable(stdout, []oauthProviderSettingsResource{settings}) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printOAuthSettingsTable(out io.Writer, settings []oauthProviderSettingsResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tENABLED\tCLIENT ID\tHAS CLIENT SECRET\tALLOWED EMAIL DOMAIN\tISSUER URL\tDISPLAY NAME")
	for _, s := range settings {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\t%v\t%s\t%s\t%s\n", s.Provider, s.Enabled, s.ClientID, s.HasClientSecret, s.AllowedEmailDomain, s.IssuerURL, s.DisplayName)
	}
	_ = tw.Flush()
}

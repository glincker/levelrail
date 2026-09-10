package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runSettingsOAuth dispatches "settings oauth <verb> [flags]" to one of
// list/set, mirroring runRegistry's own get/set-style dispatch shape for
// the API's other platform-singleton-ish settings resource.
// GET /api/v1/settings/oauth (internal/api/oauth_settings.go's
// handleListOAuthSettings) always returns every provider at once, so
// "list" reads as an array; PUT is per provider, so "set" takes one.
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
  %[1]s settings oauth list [flags]
  %[1]s settings oauth set <provider> --client-id ID --client-secret SECRET [flags]

Configures OAuth sign-in (Google, GitHub, or a generic OIDC provider).
<provider> is one of "google", "github", "oidc". A provider must already
have a client ID and secret the first time it's enabled; --client-secret
may be omitted on a later update to leave the stored secret unchanged.

Run "%[1]s settings oauth <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsOAuthList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings oauth list", "print every provider's settings as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings oauth list [flags]\n\nShows every OAuth sign-in provider's current settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetOAuthSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list oauth settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printOAuthSettingsTable(stdout, settings) })
}

func printOAuthSettingsTable(out io.Writer, settings []oauthProviderSettingsResource) {
	if len(settings) == 0 {
		_, _ = fmt.Fprintln(out, "no oauth providers")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tENABLED\tCLIENT_ID\tHAS_CLIENT_SECRET\tDISPLAY_NAME")
	for _, s := range settings {
		_, _ = fmt.Fprintf(tw, "%s\t%v\t%s\t%v\t%s\n", s.Provider, s.Enabled, s.ClientID, s.HasClientSecret, s.DisplayName)
	}
	_ = tw.Flush()
}

func runSettingsOAuthSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings oauth set", "print the updated provider settings as JSON to stdout and nothing else", stderr)
	var enabledFlag bool
	var clientID, clientSecret, allowedEmailDomain, issuerURL, displayName string
	fs.BoolVar(&enabledFlag, "enabled", true, "enable this provider (pass --enabled=false to disable without clearing stored config)")
	fs.StringVar(&clientID, "client-id", "", "OAuth client ID (required to enable)")
	fs.StringVar(&clientSecret, "client-secret", "", "OAuth client secret (required the first time this provider is enabled; omit on update to keep the stored secret)")
	fs.StringVar(&allowedEmailDomain, "allowed-email-domain", "", "restrict sign-in to this email domain (empty allows any)")
	fs.StringVar(&issuerURL, "issuer-url", "", "OIDC issuer URL (required when provider is \"oidc\")")
	fs.StringVar(&displayName, "display-name", "", "label shown on the sign-in button")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings oauth set <provider> --client-id ID --client-secret SECRET [flags]\n\nEnables/configures/disables one OAuth sign-in provider.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	provider, ok := requireOneArg(fs, stderr, prog, "settings oauth set", "provider name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.UpdateOAuthProviderSettings(context.Background(), provider, updateOAuthProviderSettingsRequest{
		Enabled:            enabledFlag,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		AllowedEmailDomain: allowedEmailDomain,
		IssuerURL:          issuerURL,
		DisplayName:        displayName,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set oauth settings for provider %q: %w", provider, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printOAuthProviderSettingsHuman(stdout, settings) })
}

func printOAuthProviderSettingsHuman(out io.Writer, s oauthProviderSettingsResource) {
	_, _ = fmt.Fprintf(out, "provider:             %s\n", s.Provider)
	_, _ = fmt.Fprintf(out, "enabled:              %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "client_id:            %s\n", s.ClientID)
	_, _ = fmt.Fprintf(out, "has_client_secret:    %v\n", s.HasClientSecret)
	_, _ = fmt.Fprintf(out, "allowed_email_domain: %s\n", s.AllowedEmailDomain)
	_, _ = fmt.Fprintf(out, "issuer_url:           %s\n", s.IssuerURL)
	_, _ = fmt.Fprintf(out, "display_name:         %s\n", s.DisplayName)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// oauthJSONUsage is every oauth subcommand's --json flag description:
// identical across list/get/set/disable since each returns the same
// settings shape.
const oauthJSONUsage = "print the settings as JSON to stdout and nothing else"

// oauthProviders is every provider GET/PUT /api/v1/settings/oauth[/{provider}]
// accepts, mirroring internal/api's isValidOAuthProvider so an unknown
// name fails fast with a clear message instead of a generic 404.
var oauthProviders = []string{apiclient.OAuthProviderGoogle, apiclient.OAuthProviderGitHub, apiclient.OAuthProviderOIDC}

func isKnownOAuthProvider(p string) bool {
	for _, known := range oauthProviders {
		if p == known {
			return true
		}
	}
	return false
}

// runOAuth dispatches "oauth <verb> [flags]" to one of
// list/get/set/disable, the same platform-singleton-settings dispatch
// shape as runRegistry and runCloudflareTunnel. Not nested under a
// "settings" parent command: no such grouping exists anywhere else in
// this CLI, every other platform-wide settings resource (registry,
// cloudflare-tunnel) is its own flat top-level command.
func runOAuth(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, oauthUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, oauthUsage(prog))
		return exitOK
	case "list":
		return runOAuthList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runOAuthGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runOAuthSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "disable":
		return runOAuthDisable(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown oauth subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, oauthUsage(prog))
		return exitUsage
	}
}

func oauthUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s oauth list [flags]                                              show every provider's sign-in settings
  %[1]s oauth get <provider> [flags]                                    show one provider's sign-in settings
  %[1]s oauth set <provider> --client-id ID [flags]                    configure and enable a provider
  %[1]s oauth disable <provider> [flags]                                disable a provider, keeping its stored config

Configures Google/GitHub/OIDC sign-in as an alternative to a password.
<provider> is one of "google", "github", or "oidc". Client secret is
write-only: it is never echoed back by "get" or "list", only a
has_client_secret boolean reports whether one is stored. "set" is a full
replace of a provider's settings, the same as "%[1]s flags set": every
flag you want kept must be passed on every call, including on a call that
only changes --enabled. Use "disable" instead to flip a provider off
while leaving its client ID, secret, and other fields untouched.

Run "%[1]s oauth <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runOAuthList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "oauth list", "print the settings as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s oauth list [flags]\n\nShows every OAuth provider's sign-in settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.ListOAuthProviderSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list oauth settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printOAuthProviderSettingsTable(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printOAuthProviderSettingsTable(out io.Writer, settings []oauthProviderSettingsResource) {
	if len(settings) == 0 {
		_, _ = fmt.Fprintln(out, "no oauth providers")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tENABLED\tCLIENT_ID\tHAS_CLIENT_SECRET")
	for _, s := range settings {
		_, _ = fmt.Fprintf(tw, "%s\t%t\t%s\t%t\n", s.Provider, s.Enabled, s.ClientID, s.HasClientSecret)
	}
	_ = tw.Flush()
}

func findOAuthProviderSettings(settings []oauthProviderSettingsResource, provider string) (oauthProviderSettingsResource, bool) {
	for _, s := range settings {
		if s.Provider == provider {
			return s, true
		}
	}
	return oauthProviderSettingsResource{}, false
}

// oauthSubcommandPrelude is get/set/disable's shared prelude: parse
// flags, require the single <provider> positional argument, validate it
// against oauthProviders, then build the API client. ok is false once
// exitCode has already been determined by an earlier step, each of which
// has already reported its own message; the caller should return
// exitCode unchanged.
func oauthSubcommandPrelude(fs *flag.FlagSet, args []string, flags apiFlagPtrs, prog, cmdLabel string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (client *Client, provider string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, flags, prog, stderr)
	if !ok {
		return nil, "", false, outputFlags{}, exitCode, false
	}
	provider, ok = requireOneArg(fs, stderr, prog, cmdLabel, "provider name")
	if !ok {
		return nil, "", false, outputFlags{}, exitUsage, false
	}
	if !isKnownOAuthProvider(provider) {
		exitCode = reportError(stdout, stderr, jsonOut, newValidationError("unknown oauth provider %q, must be one of google, github, oidc", provider))
		return nil, "", false, outputFlags{}, exitCode, false
	}
	client = apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	return client, provider, jsonOut, of, 0, true
}

func runOAuthGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "oauth get", oauthJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s oauth get <provider> [flags]\n\nShows one OAuth provider's sign-in settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, provider, jsonOut, of, exitCode, ok := oauthSubcommandPrelude(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "oauth get", stdout, stderr, lookupEnv)
	if !ok {
		return exitCode
	}

	settings, err := client.ListOAuthProviderSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get oauth settings for %q: %w", provider, err))
	}
	found, ok := findOAuthProviderSettings(settings, provider)
	if !ok {
		return reportError(stdout, stderr, jsonOut, newValidationError("oauth provider %q not found", provider))
	}

	if err := renderResult(stdout, of.Format, of.Query, found, func() { printOAuthProviderSettings(stdout, found) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printOAuthProviderSettings(out io.Writer, s oauthProviderSettingsResource) {
	_, _ = fmt.Fprintf(out, "provider:           %s\n", s.Provider)
	_, _ = fmt.Fprintf(out, "enabled:            %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "client_id:          %s\n", s.ClientID)
	_, _ = fmt.Fprintf(out, "has_client_secret:  %v\n", s.HasClientSecret)
	if s.AllowedEmailDomain != "" {
		_, _ = fmt.Fprintf(out, "allowed_email_domain: %s\n", s.AllowedEmailDomain)
	}
	if s.IssuerURL != "" {
		_, _ = fmt.Fprintf(out, "issuer_url:         %s\n", s.IssuerURL)
	}
	if s.DisplayName != "" {
		_, _ = fmt.Fprintf(out, "display_name:       %s\n", s.DisplayName)
	}
}

func runOAuthSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "oauth set", oauthJSONUsage, stderr)
	var enabledFlag bool
	var clientID, clientSecret, allowedEmailDomain, issuerURL, displayName string
	fs.BoolVar(&enabledFlag, "enabled", true, "enable sign-in with this provider (pass --enabled=false to disable while replacing the other fields)")
	fs.StringVar(&clientID, "client-id", "", "OAuth client ID (required to enable)")
	fs.StringVar(&clientSecret, "client-secret", "", "OAuth client secret; omit to keep the currently stored one (required the first time a provider is enabled)")
	fs.StringVar(&allowedEmailDomain, "allowed-email-domain", "", "restrict sign-in to this email domain, e.g. example.com (optional)")
	fs.StringVar(&issuerURL, "issuer-url", "", "OIDC issuer URL (required to enable the oidc provider)")
	fs.StringVar(&displayName, "display-name", "", "display name shown on the sign-in button (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s oauth set <provider> --client-id ID [flags]\n\nConfigures one OAuth provider, a full replace of every field. Omitting\n--client-secret keeps whatever secret is already stored.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, provider, jsonOut, of, exitCode, ok := oauthSubcommandPrelude(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "oauth set", stdout, stderr, lookupEnv)
	if !ok {
		return exitCode
	}

	settings, err := client.UpdateOAuthProviderSettings(context.Background(), provider, updateOAuthProviderSettingsRequest{
		Enabled:            enabledFlag,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		AllowedEmailDomain: allowedEmailDomain,
		IssuerURL:          issuerURL,
		DisplayName:        displayName,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set oauth settings for %q: %w", provider, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printOAuthProviderSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runOAuthDisable(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "oauth disable", oauthJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s oauth disable <provider> [flags]\n\nDisables one OAuth provider while keeping its client ID, secret, and\nother settings stored, unlike \"oauth set --enabled=false\" which would\nreplace them with whatever those flags default to.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, provider, jsonOut, of, exitCode, ok := oauthSubcommandPrelude(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "oauth disable", stdout, stderr, lookupEnv)
	if !ok {
		return exitCode
	}

	current, err := client.ListOAuthProviderSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable oauth provider %q: %w", provider, err))
	}
	existing, found := findOAuthProviderSettings(current, provider)
	if !found {
		return reportError(stdout, stderr, jsonOut, newValidationError("oauth provider %q not found", provider))
	}

	settings, err := client.UpdateOAuthProviderSettings(context.Background(), provider, updateOAuthProviderSettingsRequest{
		Enabled:            false,
		ClientID:           existing.ClientID,
		AllowedEmailDomain: existing.AllowedEmailDomain,
		IssuerURL:          existing.IssuerURL,
		DisplayName:        existing.DisplayName,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable oauth provider %q: %w", provider, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printOAuthProviderSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

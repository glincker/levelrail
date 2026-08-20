package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runDomainsCloudflareDNS dispatches "domains cloudflare-dns <verb>
// [flags]" to one of get/set/clear, mirroring runAppsLogDrain's own
// top-level dispatch shape.
func runDomainsCloudflareDNS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsCloudflareDNSUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsCloudflareDNSUsage(prog))
		return exitOK
	case "get":
		return runDomainsCloudflareDNSGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsCloudflareDNSSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsCloudflareDNSClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains cloudflare-dns subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsCloudflareDNSUsage(prog))
		return exitUsage
	}
}

func domainsCloudflareDNSUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains cloudflare-dns get [flags]                        show the current settings
  %[1]s domains cloudflare-dns set --cf-api-token TOKEN [flags]  configure and enable
  %[1]s domains cloudflare-dns clear [flags]                     disable and forget the token

Configures the Cloudflare API token (Zone:DNS:Edit scope) ACME uses for
the DNS-01 challenge, the only challenge type that can prove control of
a wildcard domain (e.g. "*.example.com"). A domain becomes wildcard-
eligible just by having a leading "*." label; there is no separate flag
for that. This is a distinct credential from Cloudflare Tunnel's own
connector token.

Run "%[1]s domains cloudflare-dns <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsCloudflareDNSGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "domains cloudflare-dns get", "print the settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains cloudflare-dns get [flags]\n\nShows the current Cloudflare DNS-01 settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	settings, err := client.GetCloudflareDNS(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get cloudflare dns settings: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, settings); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printCloudflareDNSSettings(stdout, settings)
	return exitOK
}

func runDomainsCloudflareDNSSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "domains cloudflare-dns set", "print the settings as JSON to stdout and nothing else", stderr)
	var cfAPITokenFlag string
	fs.StringVar(&cfAPITokenFlag, "cf-api-token", "", "Cloudflare API token, Zone:DNS:Edit scope (required; distinct from --token, this CLI's own bearer auth flag)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains cloudflare-dns set --cf-api-token TOKEN [flags]\n\nConfigures and enables Cloudflare DNS-01 for wildcard domains.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	if cfAPITokenFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains cloudflare-dns set requires --cf-api-token\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	settings, err := client.SetCloudflareDNS(context.Background(), updateCloudflareDNSRequest{Enabled: true, Token: cfAPITokenFlag})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set cloudflare dns settings: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, settings); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printCloudflareDNSSettings(stdout, settings)
	return exitOK
}

func runDomainsCloudflareDNSClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, jsonOutP := apiFlagSet(prog, "domains cloudflare-dns clear", "print the settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains cloudflare-dns clear [flags]\n\nDisables Cloudflare DNS-01 and forgets the stored token.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *jsonOutP

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, lookupEnv)

	settings, err := client.DisconnectCloudflareDNS(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear cloudflare dns settings: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, settings); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printCloudflareDNSSettings(stdout, settings)
	return exitOK
}

func printCloudflareDNSSettings(out io.Writer, s cloudflareDNSResource) {
	_, _ = fmt.Fprintf(out, "enabled:   %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "has_token: %v\n", s.HasToken)
}

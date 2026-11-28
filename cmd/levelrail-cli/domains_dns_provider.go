package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/glincker/levelrail/internal/apiclient"
)

var dnsTokenProviderNames = []string{"digitalocean", "hetzner", "azure"}

func runDomainsDNSProvider(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		_, _ = fmt.Fprint(stdout, domainsDNSProviderUsage(prog))
		return exitOK
	}
	if len(args) < 1 {
		_, _ = fmt.Fprint(stderr, domainsDNSProviderUsage(prog))
		return exitUsage
	}
	verb := args[0]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains dns-connect "+verb, "print the result as JSON to stdout and nothing else", stderr)
	provider := fs.String("provider", "", "digitalocean, hetzner or azure")
	credFile := fs.String("credential-file", "", "file holding the API token, or the credential JSON for azure (set only)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if !slices.Contains(dnsTokenProviderNames, *provider) {
		_, _ = fmt.Fprintf(stderr, "%s: --provider must be one of %s\n", prog, strings.Join(dnsTokenProviderNames, ", "))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		result any
		render func()
		err    error
	)
	switch verb {
	case "get", "clear", "set":
		var s apiclient.DNSTokenResource
		switch verb {
		case "get":
			s, err = client.GetDNSToken(ctx, *provider)
		case "clear":
			s, err = client.DisconnectDNSToken(ctx, *provider)
		default:
			if *credFile == "" {
				_, _ = fmt.Fprintf(stderr, "%s: domains dns-connect set requires --credential-file\n", prog)
				return exitUsage
			}
			raw, rerr := os.ReadFile(*credFile) //nolint:gosec // operator-chosen path to their own credential file
			if rerr != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("read credential file: %w", rerr))
			}
			s, err = client.SetDNSToken(ctx, *provider, apiclient.UpdateDNSTokenRequest{Enabled: true, Credential: strings.TrimSpace(string(raw))})
		}
		result = s
		render = func() {
			_, _ = fmt.Fprintf(stdout, "provider: %s\nenabled:  %v\nhas_key:  %v\n", s.Provider, s.Enabled, s.HasKey)
		}
	case "test":
		var r apiclient.DNSTokenTestResult
		r, err = client.TestDNSToken(ctx, *provider)
		result = r
		render = func() {
			if !r.OK {
				_, _ = fmt.Fprintf(stdout, "failed: %s\n", r.Error)
				return
			}
			_, _ = fmt.Fprintf(stdout, "ok: %d zone(s)\n", len(r.Zones))
			for _, z := range r.Zones {
				_, _ = fmt.Fprintf(stdout, "  %s\n", z)
			}
		}
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains dns-connect verb %q\n\n%s", prog, verb, domainsDNSProviderUsage(prog))
		return exitUsage
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s dns provider %s: %w", verb, *provider, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, result, render); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func domainsDNSProviderUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains dns-connect get --provider NAME [flags]                         show the connection
  %[1]s domains dns-connect set --provider NAME --credential-file FILE [flags]  store the credential and enable
  %[1]s domains dns-connect test --provider NAME [flags]                        list the zones the credential sees
  %[1]s domains dns-connect clear --provider NAME [flags]                       disable and forget the credential

NAME is digitalocean, hetzner or azure. DigitalOcean and Hetzner take an API
token. Azure takes a JSON file with tenant_id, client_id, client_secret,
subscription_id and resource_group (the DNS Zone Contributor role is enough).
Connects records, automatic records on domain add and ACME DNS-01 wildcards.
Cloudflare, Route53 and Google Cloud DNS take precedence if enabled.
`, prog)
}

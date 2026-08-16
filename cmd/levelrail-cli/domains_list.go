package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runDomainsList implements "domains list": GET /api/v1/domains
// (internal/api/ingress_settings.go's own handleListDomains).
// AbilityRead server-side, so the normal bearer-token
// --token/APP_API_TOKEN/credentials-file resolution every other
// non-tokens/auth command in this CLI uses applies here unchanged.
func runDomainsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" domains list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print domains as a JSON array to stdout and nothing else")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsListUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	domains, err := client.ListDomains(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list domains: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, domains); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printDomainsTable(stdout, domains)
	return exitOK
}

// printDomainsTable prints a compact, aligned table of domains, the
// same shape output.go's own printAppsTable/printDatabasesTable already
// establish.
func printDomainsTable(out io.Writer, domains []domainResource) {
	if len(domains) == 0 {
		_, _ = fmt.Fprintln(out, "no domains")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tSERVICE")
	for _, d := range domains {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", d.Domain, d.ServiceName)
	}
	_ = tw.Flush()
}

func domainsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains list [flags]

Lists every service_domains row across every app in one call.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --json                    print domains as a JSON array to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

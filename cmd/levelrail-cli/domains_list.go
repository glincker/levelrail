package main

import (
	"context"
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
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains list", "print domains as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	domains, err := client.ListDomains(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list domains: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, domains, func() { printDomainsTable(stdout, domains) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
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
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print domains as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

package main

import (
	"fmt"
	"io"
)

// runDomains dispatches "domains <verb> [flags]": "list" (a read-only
// aggregation of every app's own service_domains rows,
// internal/api/ingress_settings.go's own handleListDomains doc comment;
// domain assignment itself happens through "apps create"/an app's own
// domains: field, not through this command) and "cloudflare-dns" (the
// DNS-01 credential a wildcard domain, e.g. "*.example.com", needs:
// internal/ingress.IsWildcardDomain is the only thing that marks a
// domain wildcard-eligible, there is no separate flag for it).
func runDomains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsUsage(prog))
		return exitOK
	case "list":
		return runDomainsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "cloudflare-dns":
		return runDomainsCloudflareDNS(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsUsage(prog))
		return exitUsage
	}
}

func domainsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains list [flags]                    list every app's domains in one call
  %[1]s domains cloudflare-dns <verb> [flags]   configure wildcard-domain ACME DNS-01 via Cloudflare

Run "%[1]s domains <subcommand> -h" for a subcommand's own flags.
`, prog)
}

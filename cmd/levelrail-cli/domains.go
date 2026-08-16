package main

import (
	"fmt"
	"io"
)

// runDomains dispatches "domains <verb> [flags]". Only "list" exists
// today: GET /api/v1/domains is a read-only aggregation of every app's
// own service_domains rows (internal/api/ingress_settings.go's own
// handleListDomains doc comment), there is no route to create, delete,
// or otherwise mutate a domain directly through this endpoint, domain
// assignment happens through "apps create"/an app's own domains: field
// instead, so there is nothing else for this command to wrap.
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
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsUsage(prog))
		return exitUsage
	}
}

func domainsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains list [flags]   list every app's domains in one call

Run "%[1]s domains <subcommand> -h" for a subcommand's own flags.
`, prog)
}

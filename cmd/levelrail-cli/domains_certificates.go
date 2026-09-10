package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runDomainsCertificates implements "domains certificates": GET
// /api/v1/certificates (internal/api/certificates.go's
// handleListCertificates), every certificate in this control plane's
// certmagic storage. Flat, not a get/set/clear dispatch group like
// domains' other sub-resources: this is a read-only view, there is
// nothing to configure here (certificates are issued and renewed by the
// embedded Caddy ingress automatically).
func runDomainsCertificates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains certificates", "print certificates as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsCertificatesUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	certs, err := client.ListCertificates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list certificates: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, certs, func() { printCertificatesTable(stdout, certs) })
}

// printCertificatesTable prints a compact, aligned table, the same shape
// printDomainsTable already establishes for this file's sibling command.
func printCertificatesTable(out io.Writer, certs []certificateResource) {
	if len(certs) == 0 {
		_, _ = fmt.Fprintln(out, "no certificates")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tSTATUS\tISSUER\tNOT_AFTER")
	for _, c := range certs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Domain, c.Status, c.Issuer, c.NotAfter.Format("2006-01-02T15:04:05Z07:00"))
	}
	_ = tw.Flush()
}

func domainsCertificatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains certificates [flags]

Lists every certificate currently in this control plane's certmagic
storage, healthy or not, so expiry can be checked or scripted. Status is
"healthy", "expiring_soon", or "expired".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print certificates as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

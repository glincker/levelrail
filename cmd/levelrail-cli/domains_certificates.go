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
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]certificateResource]{
		cmdLabel:  "domains certificates",
		jsonUsage: "print certificates as a JSON array to stdout and nothing else",
		usageText: domainsCertificatesUsage(prog),
		fetch:     func(c *Client, ctx context.Context) ([]certificateResource, error) { return c.ListCertificates(ctx) },
		errVerb:   "list certificates",
		print:     printCertificatesTable,
	})
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
	return fmt.Sprintf("Usage:\n  %s domains certificates [flags]\n\nLists every certificate currently in this control plane's certmagic\nstorage, healthy or not, so expiry can be checked or scripted. Status is\n\"healthy\", \"expiring_soon\", or \"expired\".\n\nFlags:\n", prog)
}

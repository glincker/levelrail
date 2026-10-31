package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDNSProvider prints where a domain's DNS lives; mail selects the mail route.
func runDNSProvider(prog, cmd string, args []string, mail bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, cmd, cmd+" <domain>", args, 1, "a domain", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	get := c.client.DomainDNSProvider
	if mail {
		get = c.client.MailDNSProvider
	}
	res, err := get(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("detect dns provider for %s: %w", c.rest[0], err))
	}
	return c.result(res, func() { printDNSProvider(stdout, res) })
}

func printDNSProvider(w io.Writer, p apiclient.DNSHostDetection) {
	_, _ = fmt.Fprintf(w, "domain: %s\nzone: %s\nprovider: %s\nnameservers: %s\n", p.Domain, p.Zone, p.Provider, strings.Join(p.Nameservers, ", "))
	_, _ = fmt.Fprintf(w, "connectable: %t\nconnected: %t\ncan issue certificates: %t\n", p.Connectable, p.Connected, p.CanIssueCertificates)
	if p.DomainConnect.Supported {
		_, _ = fmt.Fprintf(w, "domain connect: %s%s\n", p.DomainConnect.Host, p.DomainConnect.Path)
	}
	if p.DashboardURL != "" {
		_, _ = fmt.Fprintf(w, "dashboard: %s\n", p.DashboardURL)
	}
	_, _ = fmt.Fprintf(w, "hint: %s\n", p.Hint)
}

func runMailDNSDomains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "mail dns domains", "mail dns domains", args, 0, "", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := c.client.MailDNSDomains(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list mail dns domains: %w", err))
	}
	return c.result(res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DOMAIN\tSOURCES\tPROVIDER")
		for _, d := range res.Domains {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", d.Domain, strings.Join(d.Sources, ","), d.Provider)
		}
		_ = tw.Flush()
		for _, w := range res.Warnings {
			_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
		}
	})
}

func runMailDNSApplied(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "mail dns applied", "mail dns applied <domain>", args, 1, "a domain", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	res, err := c.client.MailDNSApplied(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("list applied mail dns for %s: %w", c.rest[0], err))
	}
	return c.result(res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "APPLIED\tPROVIDER\tACTION\tNAME\tTYPE\tIDS")
		for _, e := range res.Entries {
			for _, r := range e.Records {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", e.AppliedAt, e.Provider, r.Action, r.Name, r.Type, strings.Join(r.IDs, ","))
			}
		}
		_ = tw.Flush()
	})
}

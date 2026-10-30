package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDomainDNSStatus prints each required record of an app domain with its live state.
func runDomainDNSStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var app string
	c, code, ok := parseDBAccess(prog, "domains dns-status", "domains dns-status <domain> [--app NAME]", args, 1, "a domain", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&app, "app", "", "app the domain belongs to")
	})
	if !ok {
		return code
	}
	res, err := c.client.DomainDNSStatus(context.Background(), c.rest[0], app)
	if err != nil {
		return c.fail(fmt.Errorf("dns status for %s: %w", c.rest[0], err))
	}
	return c.result(res, func() { printDNSStatus(stdout, res) })
}

// runMailDNSStatus prints each mail record with its live state.
func runMailDNSStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var host, ipv4, ipv6, sel, key, preset string
	c, code, ok := parseDBAccess(prog, "mail dns status", "mail dns status <domain> [--host H] [--ipv4 IP] [--ipv6 IP] [--dkim-selector S] [--dkim-key KEY] [--preset P]", args, 1, "a domain", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&host, "host", "", "mail host name (default mail.<domain>)")
		fs.StringVar(&ipv4, "ipv4", "", "public IPv4 address of the mail host")
		fs.StringVar(&ipv6, "ipv6", "", "public IPv6 address of the mail host")
		fs.StringVar(&sel, "dkim-selector", "", "DKIM selector")
		fs.StringVar(&key, "dkim-key", "", "DKIM public key (base64)")
		fs.StringVar(&preset, "preset", "", "hosted mail provider preset")
	})
	if !ok {
		return code
	}
	q := url.Values{}
	for k, v := range map[string]string{"mail_host": host, "ipv4": ipv4, "ipv6": ipv6, "dkim_selector": sel, "dkim_public_key": key, "preset": preset} {
		if v != "" {
			q.Set(k, v)
		}
	}
	res, err := c.client.MailDNSStatus(context.Background(), c.rest[0], q)
	if err != nil {
		return c.fail(fmt.Errorf("mail dns status for %s: %w", c.rest[0], err))
	}
	return c.result(res, func() { printDNSStatus(stdout, res) })
}

func printDNSStatus(w io.Writer, s apiclient.DNSStatusReport) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TYPE\tNAME\tSTATUS\tFOUND")
	for _, r := range s.Records {
		found := make([]string, 0, len(r.Found))
		for _, f := range r.Found {
			v := f.Type + " " + f.Value
			if f.Proxied != nil && *f.Proxied {
				v += " (proxied)"
			}
			found = append(found, v)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Type, r.Name, r.Status, strings.Join(found, " | "))
	}
	_ = tw.Flush()
	c := s.Counts
	_, _ = fmt.Fprintf(w, "source: %s  checked: %s  live %d, waiting %d, different %d, conflict %d, unknown %d of %d\n", s.ProviderSource, s.CheckedAt, c.Live, c.Waiting, c.Different, c.Conflict, c.Unknown, c.Total)
	for _, r := range s.Records {
		if r.Reason != "" {
			_, _ = fmt.Fprintf(w, "%s: %s\n", r.ID, r.Reason)
		}
	}
}

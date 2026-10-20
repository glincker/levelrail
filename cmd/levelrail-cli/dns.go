package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func dnsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s dns zones list [--provider P]                       zones the connected provider can see
  %[1]s dns zones create <domain> [--account-id ID]           create a zone and print its name servers
  %[1]s dns zones delete <zone> --confirm <zone> [--force]    delete a zone (force when it still has records)
  %[1]s dns zones nameservers <zone>                         the name servers to set at your registrar
  %[1]s dns zones verify <zone>                              check delegation through public resolvers
  %[1]s dns records list|add|update|delete|import|export|template <zone> [flags]
  %[1]s dns check <name> [--type T] [--zone Z]                answers from resolvers and the zone's own servers
  %[1]s dns health-checks list|create|delete [flags]          Route53 health checks for failover records

Zones and records live at Cloudflare or Route53, using the DNS provider
credentials connected under "%[1]s domains cloudflare-dns" or
"%[1]s domains route53-dns". <zone> is a zone id or name. --provider picks
one when both are connected.
`, prog)
}

// runDNS dispatches "dns <group> <verb>".
func runDNS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, dnsUsage(prog))
		return exitOK
	case "zones":
		return runDNSZones(prog, rest, stdout, stderr, lookupEnv)
	case "records":
		return runDNSRecords(prog, rest, stdout, stderr, lookupEnv)
	case "check":
		return runDNSCheck(prog, rest, stdout, stderr, lookupEnv)
	case "health-checks":
		return runDNSHealthChecks(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown dns subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
}

func runDNSZones(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	var provider string
	providerFlag := func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", "cloudflare or route53, when both are connected")
	}
	ctx := context.Background()
	switch args[0] {
	case "list":
		c, code, ok := parseDBAccess(prog, "dns zones list", "dns zones list [flags]", rest, 0, "", stdout, stderr, lookupEnv, providerFlag)
		if !ok {
			return code
		}
		out, err := c.client.ListDNSZones(ctx, provider)
		if err != nil {
			return c.fail(fmt.Errorf("list dns zones: %w", err))
		}
		return c.result(out, func() { printDNSZones(stdout, out) })
	case "create":
		var account string
		c, code, ok := parseDBAccess(prog, "dns zones create", "dns zones create <domain> [flags]", rest, 1, "a domain", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			providerFlag(fs)
			fs.StringVar(&account, "account-id", "", "Cloudflare account id, when the token sees more than one")
		})
		if !ok {
			return code
		}
		z, err := c.client.CreateDNSZone(ctx, provider, c.rest[0], account)
		if err != nil {
			return c.fail(fmt.Errorf("create zone %q: %w", c.rest[0], err))
		}
		return c.result(z, func() {
			_, _ = fmt.Fprintf(stdout, "zone %s created at %s (id %s)\nSet these name servers at your registrar:\n", z.Name, z.Provider, z.ID)
			for _, ns := range z.NameServers {
				_, _ = fmt.Fprintf(stdout, "  %s\n", ns)
			}
			_, _ = fmt.Fprintf(stdout, "Then run: %s dns zones verify %s\n", prog, z.Name)
		})
	case "delete":
		var confirm string
		var force bool
		c, code, ok := parseDBAccess(prog, "dns zones delete", "dns zones delete <zone> --confirm <zone> [--force]", rest, 1, "a zone", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			providerFlag(fs)
			fs.StringVar(&confirm, "confirm", "", "repeat the zone name to confirm (required)")
			fs.BoolVar(&force, "force", false, "delete even when records beyond NS and SOA remain")
		})
		if !ok {
			return code
		}
		if confirm == "" {
			_, _ = fmt.Fprintf(stderr, "%s: dns zones delete: --confirm <zone name> is required\n", prog)
			return exitUsage
		}
		if err := c.client.DeleteDNSZone(ctx, provider, c.rest[0], confirm, force); err != nil {
			return c.fail(fmt.Errorf("delete zone %q: %w", c.rest[0], err))
		}
		return c.result(map[string]string{"deleted": c.rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "zone %s deleted\n", c.rest[0]) })
	case "nameservers":
		c, code, ok := parseDBAccess(prog, "dns zones nameservers", "dns zones nameservers <zone> [flags]", rest, 1, "a zone", stdout, stderr, lookupEnv, providerFlag)
		if !ok {
			return code
		}
		ns, err := c.client.DNSZoneNameServers(ctx, provider, c.rest[0])
		if err != nil {
			return c.fail(fmt.Errorf("name servers of %q: %w", c.rest[0], err))
		}
		return c.result(ns, func() { _, _ = fmt.Fprintln(stdout, strings.Join(ns, "\n")) })
	case "verify":
		c, code, ok := parseDBAccess(prog, "dns zones verify", "dns zones verify <zone> [flags]", rest, 1, "a zone", stdout, stderr, lookupEnv, providerFlag)
		if !ok {
			return code
		}
		d, err := c.client.DNSZoneDelegation(ctx, provider, c.rest[0])
		if err != nil {
			return c.fail(fmt.Errorf("verify delegation of %q: %w", c.rest[0], err))
		}
		return c.result(d, func() { printDelegation(stdout, d) })
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown dns zones subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
}

func printDNSZones(out io.Writer, l apiclient.DNSZonesList) {
	if len(l.Zones) == 0 {
		_, _ = fmt.Fprintf(out, "no zones (provider: %s)\n", l.Provider)
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tID\tPROVIDER\tSTATUS\tRECORDS\tNAME SERVERS")
	for _, z := range l.Zones {
		count := "-"
		if z.RecordCount != nil {
			count = fmt.Sprint(*z.RecordCount)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", z.Name, z.ID, z.Provider, dashIfEmpty(z.Status), count, strings.Join(z.NameServers, ","))
	}
	_ = tw.Flush()
}

func printDelegation(out io.Writer, d apiclient.DNSDelegation) {
	_, _ = fmt.Fprintf(out, "%s: %s\nexpected: %s\n", d.Delegation.Domain, d.Delegation.State, strings.Join(d.Delegation.Expected, ", "))
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RESOLVER\tVERDICT\tNAME SERVERS")
	for _, r := range d.Delegation.Resolvers {
		ns := strings.Join(r.NameServers, ",")
		if r.Error != "" {
			ns = r.Error
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Server, r.Verdict, dashIfEmpty(ns))
	}
	_ = tw.Flush()
	if len(d.Delegation.Elsewhere) > 0 {
		_, _ = fmt.Fprintf(out, "still pointing at: %s\n", strings.Join(d.Delegation.Elsewhere, ", "))
	}
}

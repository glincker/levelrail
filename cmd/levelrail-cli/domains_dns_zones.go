package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const domainsDNSProviderHelp = "cloudflare, route53 or googleclouddns, when more than one is connected"

func runDomainsDNSZones(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var provider string
	c, code, ok := parseDBAccess(prog, "domains dns zones", "domains dns zones [--provider P]", args, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", domainsDNSProviderHelp)
	})
	if !ok {
		return code
	}
	out, err := c.client.ListDNSZones(context.Background(), provider)
	if err != nil {
		return c.fail(fmt.Errorf("list dns zones: %w", err))
	}
	return c.result(out, func() {
		if len(out.Zones) == 0 {
			_, _ = fmt.Fprintf(stdout, "no zones (provider: %s)\n", out.Provider)
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tID\tPROVIDER\tSTATUS\tDELEGATED\tNAMESERVERS")
		for _, z := range out.Zones {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", z.Name, z.ID, z.Provider, dashIfEmpty(z.Status), dashIfEmpty(z.Delegated), strings.Join(z.NameServers, ","))
		}
		_ = tw.Flush()
	})
}

func runDomainsDNSZone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var provider string
	c, code, ok := parseDBAccess(prog, "domains dns zone", "domains dns zone <id> [--provider P]", args, 1, "a zone id or name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", domainsDNSProviderHelp+" (found from the zone list when empty)")
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	if provider == "" {
		list, err := c.client.ListDNSZones(ctx, "")
		if err != nil {
			return c.fail(fmt.Errorf("list dns zones: %w", err))
		}
		for _, z := range list.Zones {
			if z.ID == c.rest[0] || strings.EqualFold(strings.TrimSuffix(c.rest[0], "."), z.Name) {
				provider = z.Provider
				break
			}
		}
		if provider == "" {
			return c.fail(errors.New("zone " + c.rest[0] + " not found at any connected provider"))
		}
	}
	z, err := c.client.GetDNSZoneDetail(ctx, provider, c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("zone %q: %w", c.rest[0], err))
	}
	return c.result(z, func() { printDomainsDNSZone(stdout, z) })
}

func printDomainsDNSZone(out io.Writer, z apiclient.DNSZone) {
	_, _ = fmt.Fprintf(out, "zone:        %s (%s, id %s)\nstatus:      %s\nnameservers: %s\ndelegated:   %s\n",
		z.Name, z.Provider, z.ID, dashIfEmpty(z.Status), strings.Join(z.NameServers, ", "), dashIfEmpty(z.Delegated))
	if z.DelegationReason != "" {
		_, _ = fmt.Fprintf(out, "reason:      %s\n", z.DelegationReason)
	}
	if len(z.CurrentNameservers) > 0 {
		_, _ = fmt.Fprintf(out, "points to:   %s\n", strings.Join(z.CurrentNameservers, ", "))
	}
	if z.RegistrarHint != nil {
		_, _ = fmt.Fprintf(out, "registrar:   %s %s\n", z.RegistrarHint.Name, z.RegistrarHint.URL)
	}
	if z.Delegated == "no" || z.Delegated == "partial" {
		_, _ = fmt.Fprintln(out, "Set the nameservers above at your registrar so the zone's records go live.")
	}
}

func runDomainsDNSZoneRecords(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var provider, typ, search string
	c, code, ok := parseDBAccess(prog, "domains dns records", "domains dns records <zone> [--type T] [--search S] [--provider P]", args, 1, "a zone id or name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", domainsDNSProviderHelp)
		fs.StringVar(&typ, "type", "", "only records of this type, such as A, CNAME or TXT")
		fs.StringVar(&search, "search", "", "only records whose name or value contains this text")
	})
	if !ok {
		return code
	}
	out, err := c.client.ListDNSZoneRecords(context.Background(), provider, c.rest[0], typ, search)
	if err != nil {
		return c.fail(fmt.Errorf("records of %q: %w", c.rest[0], err))
	}
	return c.result(out, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tTYPE\tTTL\tVALUES\tFLAGS")
		for _, rs := range out.Records {
			var flags []string
			if rs.ReadOnly {
				flags = append(flags, "read-only")
			}
			if rs.ManagedByLevelrail {
				flags = append(flags, "managed")
			}
			if rs.Proxied {
				flags = append(flags, "proxied")
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", rs.Name, rs.Type, rs.TTL, strings.Join(rs.Values, " "), dashIfEmpty(strings.Join(flags, ",")))
		}
		_ = tw.Flush()
	})
}

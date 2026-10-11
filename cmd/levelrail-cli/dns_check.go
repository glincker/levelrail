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

func runDNSCheck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var typ, zone string
	c, code, ok := parseDBAccess(prog, "dns check", "dns check <name> [--type T] [--zone Z]", args, 1, "a name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&typ, "type", "A", "record type")
		fs.StringVar(&zone, "zone", "", "zone id or name, to also ask its own name servers")
	})
	if !ok {
		return code
	}
	p, err := c.client.CheckDNS(context.Background(), c.rest[0], strings.ToUpper(typ), zone)
	if err != nil {
		return c.fail(fmt.Errorf("check %s %s: %w", c.rest[0], typ, err))
	}
	return c.result(p, func() { printPropagation(stdout, p) })
}

func printPropagation(out io.Writer, p apiclient.DNSPropagation) {
	verdict := "answers agree"
	if !p.Agree {
		verdict = "answers differ"
	}
	_, _ = fmt.Fprintf(out, "%s %s: %s\n", p.Name, p.Type, verdict)
	if len(p.Expected) > 0 {
		_, _ = fmt.Fprintf(out, "zone says: %s\n", strings.Join(p.Expected, " | "))
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SERVER\tSOURCE\tTTL\tANSWER\tMATCHES")
	for _, a := range p.Answers {
		ans := strings.Join(a.Values, " | ")
		if a.Error != "" {
			ans = a.Error
		} else if ans == "" {
			ans = "no records"
		}
		match := "-"
		if a.Matches != nil {
			match = fmt.Sprint(*a.Matches)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", a.Server, a.Source, a.TTL, ans, match)
	}
	_ = tw.Flush()
}

func runDNSHealthChecks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
	ctx := context.Background()
	rest := args[1:]
	switch args[0] {
	case "list":
		c, code, ok := parseDBAccess(prog, "dns health-checks list", "dns health-checks list [flags]", rest, 0, "", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		list, err := c.client.ListDNSHealthChecks(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("list health checks: %w", err))
		}
		return c.result(list, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tTYPE\tTARGET\tPORT\tPATH")
			for _, h := range list {
				target := h.FQDN
				if target == "" {
					target = h.IPAddress
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", h.ID, h.Type, target, h.Port, dashIfEmpty(h.ResourcePath))
			}
			_ = tw.Flush()
		})
	case "create":
		var hc apiclient.DNSHealthCheck
		var port int
		c, code, ok := parseDBAccess(prog, "dns health-checks create", "dns health-checks create --type HTTPS --fqdn F [--port P] [--path /healthz]", rest, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.StringVar(&hc.Type, "type", "HTTPS", "HTTP, HTTPS or TCP")
			fs.StringVar(&hc.FQDN, "fqdn", "", "host name to probe")
			fs.StringVar(&hc.IPAddress, "ip", "", "IP address to probe")
			fs.IntVar(&port, "port", 0, "port (TCP requires one)")
			fs.StringVar(&hc.ResourcePath, "path", "", "request path for HTTP and HTTPS")
		})
		if !ok {
			return code
		}
		hc.Port = int32(port) //nolint:gosec // the API validates the 1-65535 range
		out, err := c.client.CreateDNSHealthCheck(ctx, hc)
		if err != nil {
			return c.fail(fmt.Errorf("create health check: %w", err))
		}
		return c.result(out, func() { _, _ = fmt.Fprintf(stdout, "health check %s created\n", out.ID) })
	case "delete":
		c, code, ok := parseDBAccess(prog, "dns health-checks delete", "dns health-checks delete <id>", rest, 1, "a health check id", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		if err := c.client.DeleteDNSHealthCheck(ctx, c.rest[0]); err != nil {
			return c.fail(fmt.Errorf("delete health check %q: %w", c.rest[0], err))
		}
		return c.result(map[string]string{"deleted": c.rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "health check %s deleted\n", c.rest[0]) })
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown dns health-checks subcommand %q\n\n", prog, args[0])
		return exitUsage
	}
}

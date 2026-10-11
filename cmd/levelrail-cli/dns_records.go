package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// dnsRecordFlagSet holds the flags shared by records add and update.
type dnsRecordFlagSet struct {
	provider, name, typ, routing, setID, failover, healthCheck string
	values                                                     stringList
	ttl                                                        int
	weight                                                     int64
	proxied                                                    bool
}

func (f *dnsRecordFlagSet) register(fs *flag.FlagSet) {
	fs.StringVar(&f.provider, "provider", "", "cloudflare or route53, when both are connected")
	fs.StringVar(&f.name, "name", "@", `record name relative to the zone, "@" for the apex, "*" for a wildcard`)
	fs.StringVar(&f.typ, "type", "", "A, AAAA, CNAME, TXT, MX, CAA, SRV or NS (required)")
	fs.Var(&f.values, "value", "a value; repeat for a multi value set")
	fs.IntVar(&f.ttl, "ttl", 0, "TTL in seconds (default APP_DNS_DEFAULT_TTL or 300; 1 is Cloudflare auto)")
	fs.BoolVar(&f.proxied, "proxied", false, "proxy through Cloudflare (A, AAAA, CNAME)")
	fs.StringVar(&f.routing, "routing", "", "simple, weighted, failover or multivalue (Route53)")
	fs.StringVar(&f.setID, "set-id", "", "set identifier for a routed set (Route53)")
	fs.Int64Var(&f.weight, "weight", -1, "weight 0-255 for weighted routing")
	fs.StringVar(&f.failover, "failover", "", "PRIMARY or SECONDARY for failover routing")
	fs.StringVar(&f.healthCheck, "health-check", "", "Route53 health check id")
}

func (f *dnsRecordFlagSet) set() apiclient.DNSRecordSet {
	rs := apiclient.DNSRecordSet{
		Name: f.name, Type: strings.ToUpper(f.typ), TTL: f.ttl, Values: f.values, Proxied: f.proxied,
		Routing: f.routing, SetIdentifier: f.setID, Failover: strings.ToUpper(f.failover), HealthCheckID: f.healthCheck,
	}
	if f.weight >= 0 {
		w := f.weight
		rs.Weight = &w
	}
	return rs
}

func runDNSRecords(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
	ctx := context.Background()
	rest := args[1:]
	var rf dnsRecordFlagSet
	switch args[0] {
	case "list":
		var search string
		c, code, ok := parseDBAccess(prog, "dns records list", "dns records list <zone> [--type T] [--search S]", rest, 1, "a zone", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.StringVar(&rf.provider, "provider", "", "cloudflare or route53")
			fs.StringVar(&rf.typ, "type", "", "only this record type")
			fs.StringVar(&search, "search", "", "match name or value")
		})
		if !ok {
			return code
		}
		out, err := c.client.ListDNSZoneRecords(ctx, rf.provider, c.rest[0], rf.typ, search)
		if err != nil {
			return c.fail(fmt.Errorf("list records of %q: %w", c.rest[0], err))
		}
		return c.result(out, func() { printDNSRecordSets(stdout, out.Records) })
	case "add", "update":
		var newName string
		c, code, ok := parseDBAccess(prog, "dns records "+args[0], "dns records "+args[0]+" <zone> --type T --name N --value V [flags]", rest, 1, "a zone", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			rf.register(fs)
			if args[0] == "update" {
				fs.StringVar(&newName, "new-name", "", "rename the set")
			}
		})
		if !ok {
			return code
		}
		if rf.typ == "" || len(rf.values) == 0 {
			_, _ = fmt.Fprintf(stderr, "%s: dns records %s: --type and at least one --value are required\n", prog, args[0])
			return exitUsage
		}
		rs := rf.set()
		var out apiclient.DNSRecordWrite
		var err error
		if args[0] == "add" {
			out, err = c.client.CreateDNSZoneRecord(ctx, rf.provider, c.rest[0], rs)
		} else {
			orig := apiclient.DNSRecordKey{Name: rf.name, Type: rs.Type, SetIdentifier: rf.setID}
			if newName != "" {
				rs.Name = newName
			}
			out, err = c.client.UpdateDNSZoneRecord(ctx, rf.provider, c.rest[0], orig, rs)
		}
		if err != nil {
			return c.fail(fmt.Errorf("%s %s record %q: %w", args[0], rs.Type, rs.Name, err))
		}
		return c.result(out, func() {
			printDNSRecordSets(stdout, []apiclient.DNSRecordSet{out.Record})
			for _, is := range out.Issues {
				_, _ = fmt.Fprintf(stderr, "%s: %s\n", is.Severity, is.Message)
			}
		})
	case "delete":
		c, code, ok := parseDBAccess(prog, "dns records delete", "dns records delete <zone> --name N --type T [--set-id S]", rest, 1, "a zone", stdout, stderr, lookupEnv, rf.register)
		if !ok {
			return code
		}
		if rf.typ == "" {
			_, _ = fmt.Fprintf(stderr, "%s: dns records delete: --type is required\n", prog)
			return exitUsage
		}
		k := apiclient.DNSRecordKey{Name: rf.name, Type: strings.ToUpper(rf.typ), SetIdentifier: rf.setID}
		if err := c.client.DeleteDNSZoneRecord(ctx, rf.provider, c.rest[0], k); err != nil {
			return c.fail(fmt.Errorf("delete %s %s: %w", k.Name, k.Type, err))
		}
		return c.result(k, func() { _, _ = fmt.Fprintf(stdout, "deleted %s %s\n", k.Name, k.Type) })
	case "import":
		return runDNSRecordsImport(prog, rest, stdout, stderr, lookupEnv)
	case "export":
		return runDNSRecordsExport(prog, rest, stdout, stderr, lookupEnv)
	case "template":
		return runDNSRecordsTemplate(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown dns records subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, dnsUsage(prog))
		return exitUsage
	}
}

func printDNSRecordSets(out io.Writer, sets []apiclient.DNSRecordSet) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tTYPE\tTTL\tVALUES\tROUTING\tPROXIED")
	for _, s := range sets {
		vals := strings.Join(s.Values, " | ")
		if s.Alias != nil {
			vals = "alias " + s.Alias.DNSName
		}
		routing := dashIfEmpty(s.Routing)
		if s.SetIdentifier != "" {
			routing += " (" + s.SetIdentifier + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%t\n", s.Name, s.Type, s.TTL, vals, routing, s.Proxied)
	}
	_ = tw.Flush()
}

func printDNSPlan(out io.Writer, res apiclient.DNSImportResult) {
	for _, ch := range res.Plan.Changes {
		if ch.Action == "unchanged" {
			continue
		}
		_, _ = fmt.Fprintf(out, "%-9s %s %s %s\n", ch.Action, ch.Set.Name, ch.Set.Type, strings.Join(ch.Set.Values, " | "))
		for _, is := range ch.Issues {
			_, _ = fmt.Fprintf(out, "          %s: %s\n", is.Severity, is.Message)
		}
	}
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", w)
	}
	for _, e := range res.Errors {
		_, _ = fmt.Fprintf(out, "error: %s\n", e)
	}
	_, _ = fmt.Fprintf(out, "create %d, update %d, delete %d, unchanged %d, applied %d\n",
		res.Plan.Summary["create"], res.Plan.Summary["update"], res.Plan.Summary["delete"], res.Plan.Summary["unchanged"], res.Applied)
	if !res.Plan.Blocked && res.Applied == 0 {
		_, _ = fmt.Fprintln(out, "preview only: rerun with --apply to make these changes")
	}
}

// dnsImportFormat picks bind or json from --format or the file extension.
func dnsImportFormat(format, file string, content []byte) string {
	if format != "" {
		return strings.ToLower(format)
	}
	if strings.HasSuffix(file, ".json") || strings.HasPrefix(strings.TrimSpace(string(content)), "{") || strings.HasPrefix(strings.TrimSpace(string(content)), "[") {
		return "json"
	}
	return "bind"
}

func runDNSRecordsImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var provider, file, format, confirm string
	var replace, apply bool
	c, code, ok := parseDBAccess(prog, "dns records import", "dns records import <zone> --file F [--format bind|json] [--apply] [--replace --confirm <zone>]", args, 1, "a zone", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", "cloudflare or route53")
		fs.StringVar(&file, "file", "", `zone file or JSON export, "-" for stdin (required)`)
		fs.StringVar(&format, "format", "", "bind or json (default: from the file)")
		fs.BoolVar(&apply, "apply", false, "apply the plan (default: preview only)")
		fs.BoolVar(&replace, "replace", false, "delete record sets missing from the file")
		fs.StringVar(&confirm, "confirm", "", "zone name, required with --replace --apply")
	})
	if !ok {
		return code
	}
	if file == "" {
		_, _ = fmt.Fprintf(stderr, "%s: dns records import: --file is required\n", prog)
		return exitUsage
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(stdinSource)
	} else {
		data, err = os.ReadFile(file) //nolint:gosec // the operator names the file to import
	}
	if err != nil {
		return c.fail(fmt.Errorf("read %s: %w", file, err))
	}
	req := apiclient.DNSImportRequest{Format: dnsImportFormat(format, file, data), Content: string(data), Replace: replace, Apply: apply, Confirm: confirm}
	res, err := c.client.ImportDNSZoneRecords(context.Background(), provider, c.rest[0], req)
	if err != nil {
		return c.fail(fmt.Errorf("import into %q: %w", c.rest[0], err))
	}
	return c.result(res, func() { printDNSPlan(stdout, res) })
}

func runDNSRecordsExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var provider, format, outFile string
	c, code, ok := parseDBAccess(prog, "dns records export", "dns records export <zone> [--format bind|json] [--out F]", args, 1, "a zone", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", "cloudflare or route53")
		fs.StringVar(&format, "format", "bind", "bind or json")
		fs.StringVar(&outFile, "out", "", "write to this file instead of stdout")
	})
	if !ok {
		return code
	}
	data, err := c.client.ExportDNSZoneRecords(context.Background(), provider, c.rest[0], format)
	if err != nil {
		return c.fail(fmt.Errorf("export %q: %w", c.rest[0], err))
	}
	if outFile == "" {
		_, _ = stdout.Write(data)
		return exitOK
	}
	if err := os.WriteFile(outFile, data, 0o600); err != nil {
		return c.fail(fmt.Errorf("write %s: %w", outFile, err))
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", outFile)
	return exitOK
}

func runDNSRecordsTemplate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var provider string
	var apply bool
	params := stringMapFlag{}
	c, code, ok := parseDBAccess(prog, "dns records template", "dns records template <zone> <template> [--param k=v]... [--apply]", args, 2, "a zone and a template id (email, dkim, verification, www-to-apex, google-workspace, microsoft-365, caa-letsencrypt)", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&provider, "provider", "", "cloudflare or route53")
		fs.Var(params, "param", "template parameter KEY=VALUE, repeatable")
		fs.BoolVar(&apply, "apply", false, "apply the plan (default: preview only)")
	})
	if !ok {
		return code
	}
	res, err := c.client.ApplyDNSTemplate(context.Background(), provider, c.rest[0], c.rest[1], params, apply)
	if err != nil {
		return c.fail(fmt.Errorf("template %q on %q: %w", c.rest[1], c.rest[0], err))
	}
	return c.result(res, func() { printDNSPlan(stdout, res) })
}

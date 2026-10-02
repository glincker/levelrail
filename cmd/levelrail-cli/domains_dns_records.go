package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// domainsDNSJSONUsage is every domains dns subcommand's --json flag
// description, mirroring domainsWAFJSONUsage's shape for a different
// per-domain resource.
const domainsDNSJSONUsage = "print the dns records result as JSON to stdout and nothing else"

// runDomainsDNS dispatches "domains dns <verb> [flags]" to one of
// list/add/remove: editing a record in place is a dashboard-only
// action (DomainDnsRecordsControl), since an edit is really a delete
// plus an add under the hood and the CLI exposes those two primitives
// directly instead of a third verb that just composes them.
func runDomainsDNS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsDNSUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsDNSUsage(prog))
		return exitOK
	case "list":
		return runDomainsDNSList(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runDomainsDNSAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runDomainsDNSRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains dns subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsDNSUsage(prog))
		return exitUsage
	}
}

func domainsDNSUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains dns list <app> <domain> [flags]                             list every record in the domain's zone
  %[1]s domains dns add <app> <domain> --type T --name N --value V [flags]    append a new record
  %[1]s domains dns remove <app> <domain> --type T --name N --value V [flags] delete one exact record

Reads and writes the real A/AAAA/CNAME/TXT/MX/SRV/CAA records in one of
<app>'s domains' best-effort DNS zone, via whichever ACME DNS-01
provider (Cloudflare DNS or Route53 DNS, "%[1]s domains cloudflare-dns"/
"%[1]s domains route53-dns") is configured and enabled. "remove" matches
a record exactly: --type, --name, and --value must all equal an
existing record.

Run "%[1]s domains dns <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// domainsDNSResolveArgs collapses the parseAPIFlags/requireArgs/
// apiClientFromFlags preamble every domains dns subcommand below needs,
// mirroring domainWAFResolveArgs's identical shape.
func domainsDNSResolveArgs(
	fs *flag.FlagSet, args []string, ptrs apiFlagPtrs,
	prog, cmdLabel string, stderr io.Writer, lookupEnv func(string) (string, bool),
) (client *Client, appName, domain string, jsonOut bool, of outputFlags, exitCode int, ok bool) {
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, ptrs, prog, stderr)
	if !ok {
		return nil, "", "", false, outputFlags{}, exitCode, false
	}

	rest, argsOK := requireArgs(fs, stderr, prog, cmdLabel, "an app name and a domain", 2)
	if !argsOK {
		return nil, "", "", false, outputFlags{}, exitUsage, false
	}

	client = apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	return client, rest[0], rest[1], jsonOut, of, 0, true
}

func runDomainsDNSList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains dns list", domainsDNSJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains dns list <app> <domain> [flags]\n\nLists every record in the domain's best-effort zone.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, appName, domain, jsonOut, of, exitCode, ok := domainsDNSResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains dns list", stderr, lookupEnv)
	if !ok {
		return exitCode
	}

	result, err := client.ListDNSRecords(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list dns records for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printDNSRecordsHuman(stdout, result) })
}

// dnsRecordFlags registers the --type/--name/--value/--ttl flags shared
// by "add" and "remove", since both identify or describe a record the
// same way.
func dnsRecordFlags(fs *flag.FlagSet) (typ, name, value *string, ttl *int) {
	typ = fs.String("type", "", "record type: A, AAAA, CNAME, TXT, MX, SRV, or CAA (required)")
	name = fs.String("name", "@", `record name relative to the zone, "@" for the zone apex`)
	value = fs.String("value", "", "record value/data (required)")
	ttl = fs.Int("ttl", 300, "TTL in seconds")
	return typ, name, value, ttl
}

func runDomainsDNSAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains dns add", domainsDNSJSONUsage, stderr)
	typeFlag, nameFlag, valueFlag, ttlFlag := dnsRecordFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains dns add <app> <domain> --type T --name N --value V [flags]\n\nAppends a new record to the domain's zone, leaving any existing record with the same name and type untouched.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, appName, domain, jsonOut, of, exitCode, ok := domainsDNSResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains dns add", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	if *typeFlag == "" || *valueFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains dns add: --type and --value are required\n", prog)
		return exitUsage
	}

	result, err := client.CreateDNSRecord(context.Background(), appName, domain, apiclient.DNSRecordResource{
		Type: *typeFlag, Name: *nameFlag, Value: *valueFlag, TTLSeconds: *ttlFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("add dns record to domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printDNSRecordsHuman(stdout, result) })
}

func runDomainsDNSRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains dns remove", domainsDNSJSONUsage, stderr)
	typeFlag, nameFlag, valueFlag, ttlFlag := dnsRecordFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains dns remove <app> <domain> --type T --name N --value V [flags]\n\nDeletes one exact record: --type, --name, and --value must all match an existing record. Idempotent.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, appName, domain, jsonOut, of, exitCode, ok := domainsDNSResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, "domains dns remove", stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	if *typeFlag == "" || *valueFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains dns remove: --type and --value are required\n", prog)
		return exitUsage
	}

	result, err := client.DeleteDNSRecord(context.Background(), appName, domain, apiclient.DNSRecordResource{
		Type: *typeFlag, Name: *nameFlag, Value: *valueFlag, TTLSeconds: *ttlFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove dns record from domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printDNSRecordsHuman(stdout, result) })
}

func printDNSRecordsHuman(out io.Writer, r apiclient.DNSRecordsResponse) {
	_, _ = fmt.Fprintf(out, "domain:    %s\nprovider:  %s\nzone:      %s\n", r.Domain, r.Provider, r.Zone)
	if len(r.Records) == 0 {
		_, _ = fmt.Fprint(out, "records:   (none)\n")
		return
	}
	_, _ = fmt.Fprint(out, "records:\n")
	for _, rec := range r.Records {
		_, _ = fmt.Fprintf(out, "  %-6s %-24s %-32s ttl=%ds status=%s\n", rec.Type, rec.Name, rec.Value, rec.TTLSeconds, rec.Status)
	}
}

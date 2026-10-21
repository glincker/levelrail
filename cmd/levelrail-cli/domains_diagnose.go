package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	doctorFixAuto   = "auto"
	doctorActionReq = "request"
)

// runDomainsDiagnose implements "domains diagnose <domain> [--fix]".
func runDomainsDiagnose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains diagnose", "print the diagnosis as JSON to stdout and nothing else", stderr)
	var fix bool
	fs.BoolVar(&fix, "fix", false, "apply every fix the control plane can do itself (for example create the DNS record), then diagnose again")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsDiagnoseUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains diagnose", "a domain", 1)
	if !ok {
		return exitUsage
	}
	domain := strings.ToLower(rest[0])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	rep, err := client.DomainDiagnose(ctx, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose %q: %w", domain, err))
	}
	var applied []string
	if fix {
		applied, err = applyAutoFixes(ctx, client, rep)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		if len(applied) > 0 {
			if rep, err = client.DomainDiagnose(ctx, domain); err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose %q after fixing: %w", domain, err))
			}
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() {
		for _, a := range applied {
			_, _ = fmt.Fprintf(stdout, "applied: %s\n", a)
		}
		printDiagnosis(stdout, rep)
	})
}

// applyAutoFixes runs each failing check's auto fix once, in report order.
func applyAutoFixes(ctx context.Context, client *Client, rep apiclient.DomainDoctorReport) ([]string, error) {
	var applied []string
	for _, c := range prioritizedChecks(rep.Checks) {
		f := c.Fix
		if f == nil || f.Kind != doctorFixAuto || f.Action == nil || f.Action.Kind != doctorActionReq {
			continue
		}
		if err := client.ApplyDoctorAction(ctx, *f.Action); err != nil {
			return applied, fmt.Errorf("fix %s: %w", c.ID, err)
		}
		applied = append(applied, f.Action.Label+" ("+c.ID+")")
	}
	return applied, nil
}

func printDiagnosis(out io.Writer, rep apiclient.DomainDoctorReport) {
	printDoctorReport(out, rep)
	for _, c := range prioritizedChecks(rep.Checks) {
		if c.Fix == nil || len(c.Fix.Records) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(out, "\nrecord for %s:\n", c.ID)
		printRecords(out, c.Fix.Records, "  ")
	}
	if p := rep.Propagation; p != nil {
		_, _ = fmt.Fprintf(out, "\npropagation: %s, %s\n", p.State, p.Detail)
		if p.NextCheckSeconds > 0 {
			_, _ = fmt.Fprintf(out, "check again in %s\n", (time.Duration(p.NextCheckSeconds) * time.Second).String())
		}
	}
}

func printRecords(out io.Writer, recs []apiclient.DoctorRecord, indent string) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, r := range recs {
		_, _ = fmt.Fprintf(tw, "%s%s\t%s\t%s\n", indent, r.Type, r.Name, r.Value)
	}
	_ = tw.Flush()
}

type domainsAddResult struct {
	App     string                   `json:"app"`
	Changed bool                     `json:"changed"`
	Domains []domainsAddDomainResult `json:"domains"`
}

type domainsAddDomainResult struct {
	Domain      string                       `json:"domain"`
	Records     []apiclient.DoctorRecord     `json:"records"`
	Propagation *apiclient.DoctorPropagation `json:"propagation,omitempty"`
	Status      string                       `json:"status,omitempty"`
}

// runDomainsAdd implements "domains add <app> <domain>...": add, then print the exact DNS records.
func runDomainsAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains add", "print the result as JSON to stdout and nothing else", stderr)
	var dnsFlag string
	fs.StringVar(&dnsFlag, "dns", "", "automatic DNS record: auto (default), off or preview")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsAddUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: domains add requires an app name and at least one domain\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	app, wanted := rest[0], normalizeDomains(rest[1:])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	edited, err := client.EditAppDomains(ctx, app, apiclient.EditDomainsRequest{Add: wanted, DNS: dnsFlag})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("add domains to app %q, nothing was changed: %w", app, err))
	}
	result := domainsAddResult{App: app, Changed: edited.Changed}
	for _, d := range wanted {
		entry := domainsAddDomainResult{Domain: d, Records: []apiclient.DoctorRecord{}}
		rep, derr := client.DomainDiagnose(ctx, d)
		if derr == nil {
			entry.Records, entry.Propagation, entry.Status = rep.Records, rep.Propagation, rep.Status
		}
		result.Domains = append(result.Domains, entry)
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		for _, d := range result.Domains {
			_, _ = fmt.Fprintf(stdout, "%s added to app %q\n", d.Domain, app)
			if len(d.Records) == 0 {
				_, _ = fmt.Fprintln(stdout, "  no record to show: this server's public address is unknown, set APP_PUBLIC_HOST")
				continue
			}
			_, _ = fmt.Fprintln(stdout, "  create this record at your DNS provider (skip it if a connected provider already did):")
			printRecords(stdout, d.Records, "    ")
			_, _ = fmt.Fprintf(stdout, "  then run: %s domains diagnose %s\n", prog, d.Domain)
		}
	})
}

func domainsDiagnoseUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains diagnose <domain> [flags]

Answers "why isn't my domain working": DNS against every resolver, Cloudflare
proxy detection, wildcard records, CAA, ports 80 and 443, the certificate and
the last certificate authority error in plain language, plus whether DNS has
finished propagating and when to check again. Each problem comes with the fix:
the exact record to create, or an action the control plane can run itself.
<domain> must already belong to an app or be a standalone domain.

Flags:
  --fix                     apply every fix the control plane can do itself, then diagnose again
  --token string            API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the diagnosis as JSON to stdout, nothing else
  --output string           output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string            JMESPath expression to filter the result before printing
  -h, --help                show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func domainsAddUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains add <app> <domain>... [flags]

Adds the domains to <app> and prints the exact DNS record each one needs: an
address record at a zone apex, a CNAME to this server's hostname for a
subdomain. With a connected Cloudflare or Route 53 account the record is
created for you. Run "%[1]s domains diagnose <domain>" to watch it propagate.

Flags:
  --dns string              automatic DNS record: auto (default), off or preview
  --token string            API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string           output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string            JMESPath expression to filter the result before printing
  -h, --help                show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

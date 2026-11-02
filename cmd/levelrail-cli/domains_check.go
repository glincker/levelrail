package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Top-level verb rather than get/set/clear: a single read-only DNS check, nothing to configure.
func runDomainsCheck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains check", "print the check result as JSON to stdout and nothing else", stderr)
	var cutover bool
	var expectIPs stringList
	fs.BoolVar(&cutover, "cutover", false, "verify domains that are moving here: DNS, proxy, redirect, certificate, HSTS, response and www/apex, with a PASS, WARN or FAIL")
	fs.Var(&expectIPs, "expect-ip", "with --cutover: the address the domain must resolve to (repeatable), instead of this host's own")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsCheckUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if cutover {
		return runDomainsVerify(fs, stdout, stderr, verifyArgs{prog: prog, apiURL: apiURLFlag, token: tokenFlag, profile: profileFlag, jsonOut: jsonOut, of: of, expectIPs: expectIPs}, lookupEnv)
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains check", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.CheckDomain(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check domain %q for app %q: %w", domain, appName, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printDomainCheckHuman(stdout, result) })
}

func printDomainCheckHuman(out io.Writer, r domainCheckResource) {
	_, _ = fmt.Fprintf(out, "domain:   %s\n", r.Domain)
	_, _ = fmt.Fprintf(out, "status:   %s\n", r.Status)
	if r.ExpectedHost != "" {
		inferred := ""
		if r.HostInferred {
			inferred = " (best guess, configure APP_PUBLIC_HOST for accuracy)"
		}
		_, _ = fmt.Fprintf(out, "expected: %s%s\n", r.ExpectedHost, inferred)
	}
	if len(r.ExpectedIPv4) > 0 {
		_, _ = fmt.Fprintf(out, "expected ipv4: %v\n", r.ExpectedIPv4)
	}
	if len(r.ExpectedIPv6) > 0 {
		_, _ = fmt.Fprintf(out, "expected ipv6: %v\n", r.ExpectedIPv6)
	}
	if len(r.ResolvedHosts) > 0 {
		_, _ = fmt.Fprintf(out, "resolved: %v\n", r.ResolvedHosts)
	}
	printDomainDNSHuman(out, r)
	printDomainWizardHuman(out, r)
}

func printDomainDNSHuman(out io.Writer, r domainCheckResource) {
	if r.Verdict == "" {
		return
	}
	_, _ = fmt.Fprintf(out, "dns verdict: %s\n", r.Verdict)
	for _, g := range []struct {
		label string
		list  []apiclient.DNSSource
	}{{"authoritative", r.Authoritative}, {"public", r.Public}, {"this server", r.Local}} {
		for _, s := range g.list {
			answer := strings.Join(s.Addresses, ", ")
			if answer == "" {
				answer = s.Error
			}
			ttl := ""
			if s.TTL > 0 {
				ttl = fmt.Sprintf(" (ttl %ds)", s.TTL)
			}
			_, _ = fmt.Fprintf(out, "  %-13s %-28s %s%s\n", g.label, s.Name, answer, ttl)
		}
	}
	if r.RetryAt != "" {
		_, _ = fmt.Fprintf(out, "this server's resolver is stale, retry after %s\n", r.RetryAt)
	}
	if r.FlushCommand != "" {
		_, _ = fmt.Fprintf(out, "flush this server's DNS cache: %s\n", r.FlushCommand)
	}
	for _, h := range r.Hints {
		_, _ = fmt.Fprintf(out, "hint: %s\n", h.Message)
	}
}

func domainsCheckUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains check <app> <domain> [flags]
  %[1]s domains check --cutover <domain>... [--expect-ip <ip>]... [flags]

Runs a real DNS lookup for <domain> and reports whether it currently
resolves to this control plane's own advertised address: one of
"connected", "not_resolving", "resolves_elsewhere", "propagating" or
"unconfigured" (no APP_PUBLIC_HOST and no usable request host to infer one
from). The zone's own name servers decide first, so a stale cache on this
server never holds a correct record back; answers are printed per source
(authoritative, public, this server) with TTLs, plus a retry time and a
cache flush command when this server's resolver is behind.
<domain> must already be one of <app>'s configured domains.

With --cutover the app name is dropped and any number of domains is verified
for a move onto this server. Each gets a PASS, WARN or FAIL line, with the
next step, for DNS against this host's public addresses (or --expect-ip),
proxy in front, the http to https redirect, certificate issuer and days left,
HSTS, response status and time to first byte as seen from this host, and the
www or apex partner. It only reads. The name is requested only once DNS
points here or at a known proxy. The exit code is 1 when any domain has a FAIL.

Flags:
  --cutover                verify domains moving here (see above)
  --expect-ip string       with --cutover: required address, repeatable
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the check result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}

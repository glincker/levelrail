package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// acmeActionText turns an acme_failure.action code into the concrete step.
func acmeActionText(action string) string {
	switch action {
	case "open_port_80":
		return "open port 80 (and 443) to the internet in your firewall, router port forward and cloud security group, then retry"
	case "fix_dns":
		return "point the domain's A/AAAA record at this server, wait for DNS to propagate, then retry"
	case "wait_rate_limit":
		return "the CA is rate limiting this hostname; wait an hour or more before retrying, and test with staging"
	case "fix_caa":
		return "remove or update the CAA record so it allows the certificate authority"
	}
	return "read the ingress logs for the full CA response"
}

func challengeText(challenge string, private bool) string {
	switch {
	case challenge == "dns-01-required" && private:
		return "dns-01 required: this server's address is private, so HTTP-01 cannot validate. Enable a DNS-01 provider and use a wildcard domain"
	case challenge == "dns-01-required":
		return "dns-01 required"
	case challenge == "http-01":
		return "http-01 (ports 80 and 443 must reach this server)"
	}
	return challenge
}

func dnsProviderText(p string) string {
	switch p {
	case "cloudflare", "route53":
		return p
	}
	return "none (set one up with \"domains cloudflare-dns\" or \"domains route53-dns\")"
}

func printDomainWizardHuman(out io.Writer, r domainCheckResource) {
	if r.Challenge != "" {
		_, _ = fmt.Fprintf(out, "challenge: %s\n", challengeText(r.Challenge, r.ExpectedPrivate))
	}
	if r.DNSProvider != "" {
		_, _ = fmt.Fprintf(out, "dns-01 provider: %s\n", dnsProviderText(r.DNSProvider))
	}
	if f := r.ACMEFailure; f != nil {
		_, _ = fmt.Fprintf(out, "certificate error: %s\n", f.Error)
		_, _ = fmt.Fprintf(out, "next step: %s\n", acmeActionText(f.Action))
	}
}

// runDomainsConnectivity implements "domains connectivity": GET
// /api/v1/ingress/connectivity, whether ports 80 and 443 reach this node
// and whether HTTP-01 can work at all.
func runDomainsConnectivity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains connectivity", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsConnectivityUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetIngressConnectivity(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check ingress connectivity: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printConnectivityHuman(stdout, res) })
}

func printConnectivityHuman(out io.Writer, r apiclient.IngressConnectivity) {
	_, _ = fmt.Fprintf(out, "host:      %s\n", valueOrDash(r.Host))
	_, _ = fmt.Fprintf(out, "addresses: %s\n", joinOrDash(r.Addresses))
	for _, p := range r.Ports {
		state := "closed or filtered"
		if p.Reachable {
			state = "open"
		}
		_, _ = fmt.Fprintf(out, "port %-4d  %s\n", p.Port, state)
	}
	_, _ = fmt.Fprintf(out, "dns-01 provider: %s\n", dnsProviderText(r.DNSProvider))
	_, _ = fmt.Fprintln(out, connectivityGuidanceText(r))
}

func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func connectivityGuidanceText(r apiclient.IngressConnectivity) string {
	switch r.Guidance {
	case "no_host":
		return "No public address is known. Set APP_PUBLIC_HOST on the control plane."
	case "private_address":
		return "This server's address is private (LAN/NAT), so HTTP-01 cannot work. Use DNS-01: run \"domains cloudflare-dns\" or \"domains route53-dns\" to add a provider, then use a wildcard domain such as *.example.com."
	case "ports_unreachable":
		return "Port 80 or 443 did not answer from here. Open them in your firewall or forward them from your router, then run this again."
	}
	return "Ports 80 and 443 answer from the control plane. This does not prove they are open to the internet; \"domains check\" shows the CA's own result."
}

func domainsConnectivityUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains connectivity [flags]

Dials this node's advertised address on ports 80 and 443 from the control
plane and reports whether HTTP-01 certificate validation can work. A
private (LAN or NAT) address is decisive: use DNS-01 instead.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

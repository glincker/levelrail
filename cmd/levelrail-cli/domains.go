package main

import (
	"fmt"
	"io"
)

// Wildcard domains need an ACME DNS-01 provider (cloudflare-dns or route53-dns); only one is active, Cloudflare first.
func runDomains(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsUsage(prog))
		return exitOK
	case "list":
		return runDomainsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "cloudflare-dns":
		return runDomainsCloudflareDNS(prog, args[1:], stdout, stderr, lookupEnv)
	case "route53-dns":
		return runDomainsRoute53DNS(prog, args[1:], stdout, stderr, lookupEnv)
	case "basic-auth":
		return runDomainsBasicAuth(prog, args[1:], stdout, stderr, lookupEnv)
	case "auth-gate":
		return runDomainsAuthGate(prog, args[1:], stdout, stderr, lookupEnv)
	case "maintenance":
		return runDomainsMaintenance(prog, args[1:], stdout, stderr, lookupEnv)
	case "redirect":
		return runDomainsRedirect(prog, args[1:], stdout, stderr, lookupEnv)
	case "tls-cert":
		return runDomainsTLSCert(prog, args[1:], stdout, stderr, lookupEnv)
	case "check":
		return runDomainsCheck(prog, args[1:], stdout, stderr, lookupEnv)
	case "doctor":
		return runDomainsDoctor(prog, args[1:], stdout, stderr, lookupEnv)
	case "diagnose":
		return runDomainsDiagnose(prog, args[1:], stdout, stderr, lookupEnv)
	case "add":
		return runDomainsAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "summary":
		return runDomainsSummary(prog, args[1:], stdout, stderr, lookupEnv)
	case "activity":
		return runDomainsActivity(prog, args[1:], stdout, stderr, lookupEnv)
	case "connectivity":
		return runDomainsConnectivity(prog, args[1:], stdout, stderr, lookupEnv)
	case "cert":
		return runDomainsCert(prog, args[1:], stdout, stderr, lookupEnv)
	case "certificates":
		return runDomainsCertificates(prog, args[1:], stdout, stderr, lookupEnv)
	case "waf":
		return runDomainsWAF(prog, args[1:], stdout, stderr, lookupEnv)
	case "error-pages":
		return runDomainsErrorPages(prog, args[1:], stdout, stderr, lookupEnv)
	case "search-visibility":
		return runDomainsSearchVisibility(prog, args[1:], stdout, stderr, lookupEnv)
	case "dns":
		return runDomainsDNS(prog, args[1:], stdout, stderr, lookupEnv)
	case "go-live":
		return runDomainsGoLive(prog, args[1:], stdout, stderr, lookupEnv)
	case "backfill-base-domain":
		return runDomainsBackfillBaseDomain(prog, args[1:], stdout, stderr, lookupEnv)
	case "headers":
		return runDomainsHeaders(prog, args[1:], stdout, stderr, lookupEnv)
	case "forwarders":
		return runDomainsForwarders(prog, args[1:], stdout, stderr, lookupEnv)
	case "geo":
		return runDomainsGeo(prog, args[1:], stdout, stderr, lookupEnv)
	case "shield":
		return runDomainsShield(prog, args[1:], stdout, stderr, lookupEnv)
	case "cache":
		return runDomainsCache(prog, args[1:], stdout, stderr, lookupEnv)
	case "redirects":
		return runDomainsRedirects(prog, args[1:], stdout, stderr, lookupEnv)
	case "standalone":
		return runDomainsStandalone(prog, args[1:], stdout, stderr, lookupEnv)
	case "ports":
		return runDomainsPorts(prog, args[1:], stdout, stderr, lookupEnv)
	case "provider":
		return runDomainsProvider(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsUsage(prog))
		return exitUsage
	}
}

func domainsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains list [flags]                    list every app's domains in one call
  %[1]s domains cloudflare-dns <verb> [flags]   configure wildcard-domain ACME DNS-01 via Cloudflare
  %[1]s domains route53-dns <verb> [flags]      configure wildcard-domain ACME DNS-01 via Route53
  %[1]s domains basic-auth <verb> [flags]       protect an app's domain with HTTP Basic Auth
  %[1]s domains auth-gate <verb> [flags]        check every request with your auth service or built-in sign-in
  %[1]s domains maintenance <verb> [flags]      take one of an app's domains in or out of maintenance mode
  %[1]s domains redirect <verb> [flags]         point one of an app's domains at a target URL
  %[1]s domains tls-cert <verb> [flags]         upload or clear a domain's own (BYO) TLS certificate
  %[1]s domains check <app> <domain> [flags]    run a real DNS lookup and report whether the domain reaches this control plane
  %[1]s domains doctor <app> <domain> [flags]   run every domain check and print a prioritised fix list
  %[1]s domains diagnose <domain> [flags]       why isn't my domain working: every check with the exact fix, optionally applied with --fix
  %[1]s domains add <app> <domain>... [flags]   add domains and print the exact DNS records to create
  %[1]s domains summary [flags]                count domains by status and certificates needing attention
  %[1]s domains activity <domain> [flags]       list a domain's recent changes and certificate events
  %[1]s domains connectivity [flags]            check whether ports 80/443 reach this node and whether HTTP-01 can work
  %[1]s domains cert <domain> [--staging] [--wait]  get a real Let's Encrypt certificate for a domain, with a preflight
  %[1]s domains certificates [flags]            list every certificate in certmagic storage, healthy or not
  %[1]s domains waf <verb> [flags]              configure a domain's opt-in WAF and rate limiting
  %[1]s domains error-pages <verb> [flags]      configure a domain's custom error pages (404, 500, 502, 503)
  %[1]s domains search-visibility <app> <domain> [--hide|--show]  hide a domain from search engines
  %[1]s domains dns <verb> [flags]              list/add/remove the real DNS records in a domain's zone
  %[1]s domains go-live <app> <domain> [flags]  create the DNS record and watch the domain go live
  %[1]s domains go-live runs|undo [flags]       automation history and one-step undo
  %[1]s domains backfill-base-domain [flags]    give existing apps without a domain <app>.<base domain>
  %[1]s domains standalone <verb> [flags]       proxy a domain to a port, address or URL without an app
  %[1]s domains provider <verb> [flags]       choose the free wildcard-DNS provider for automatic hostnames
%[2]s
Run "%[1]s domains <subcommand> -h" for a subcommand's own flags.
`, prog, domainsPoliciesUsageLines(prog))
}

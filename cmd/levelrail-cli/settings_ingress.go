package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runSettingsIngress dispatches "settings ingress <verb> [flags]" to one
// of get/set, the singleton primary-domain/ACME configuration
// (internal/api/ingress_settings.go). This is the CLI counterpart of the
// dashboard's own Domains page ingress card; a domain check
// (GET /api/v1/settings/ingress/check) stays dashboard-only for now,
// since it exists to render a visual DNS status, not to script setup.
func runSettingsIngress(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsIngressUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsIngressUsage(prog))
		return exitOK
	case "get":
		return runSettingsIngressGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsIngressSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "https":
		return runSettingsIngressHTTPS(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ingress subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsIngressUsage(prog))
		return exitUsage
	}
}

func settingsIngressUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ingress get [flags]
  %[1]s settings ingress set [--primary-domain DOMAIN] [--acme-enabled] [--acme-email EMAIL] [--hsts-enabled] [--fallback-domains=false] [--public-https-port N] [--tls-terminated-upstream] [--apps-base-domain HOST] [--dns-cname-target HOST] [--dns-ttl SECONDS] [--dns-proxied] [flags]
  %[1]s settings ingress https [status] | enable --email EMAIL [--staging] [--wait] [flags]

Configures the platform-wide primary domain and ACME (Let's Encrypt)
certificate automation. --acme-email is required whenever --acme-enabled
is set. --hsts-enabled sends Strict-Transport-Security; only enable it
once a real, browser-trusted certificate is issuing (see
docs/domains-and-ingress.md).

Run "%[1]s settings ingress <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsIngressGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[ingressSettingsResource]{
		cmdLabel:  "settings ingress get",
		jsonUsage: "print the ingress settings as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s settings ingress get [flags]\n\nShows the current primary domain and ACME settings.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (ingressSettingsResource, error) {
			return c.GetIngressSettings(ctx)
		},
		errVerb: "get ingress settings",
		print:   printIngressSettingsHuman,
	})
}

func runSettingsIngressSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress set", "print the updated ingress settings as JSON to stdout and nothing else", stderr)
	var primaryDomain, acmeEmail, acmeDirectoryURL string
	var acmeEnabled, hstsEnabled, fallbackDomains, tlsUpstream bool
	var publicHTTPSPort, dnsTTL int
	var appsBaseDomain, dnsCNAMETarget string
	var dnsProxied bool
	fs.StringVar(&primaryDomain, "primary-domain", "", "hostname the dashboard itself is reachable at")
	fs.BoolVar(&acmeEnabled, "acme-enabled", false, "enable automatic TLS certificate issuance/renewal")
	fs.StringVar(&acmeEmail, "acme-email", "", "ACME account contact address (required when --acme-enabled is set)")
	fs.StringVar(&acmeDirectoryURL, "acme-directory-url", "", "ACME directory URL override (empty uses Caddy's own default, Let's Encrypt production)")
	fs.BoolVar(&hstsEnabled, "hsts-enabled", false, "send Strict-Transport-Security (only once a real, browser-trusted certificate is issuing)")
	fs.BoolVar(&fallbackDomains, "fallback-domains", true, "give apps without a domain an automatic <app>.<dashed-ip>.sslip.io hostname")
	fs.IntVar(&publicHTTPSPort, "public-https-port", 0, "port clients use when a proxy fronts this ingress (usually 443); 0 means the ingress listen port")
	fs.BoolVar(&tlsUpstream, "tls-terminated-upstream", false, "a proxy in front owns TLS: never run ACME here and build links with the public port")
	fs.StringVar(&appsBaseDomain, "apps-base-domain", "", "give new apps without a domain <app>.<base> and create its DNS record (empty clears)")
	fs.StringVar(&dnsCNAMETarget, "dns-cname-target", "", "create CNAME records to this host instead of A/AAAA to this server's address (empty clears)")
	fs.IntVar(&dnsTTL, "dns-ttl", 0, "TTL in seconds for records Levelrail creates; 0 is the provider's automatic TTL")
	fs.BoolVar(&dnsProxied, "dns-proxied", false, "turn Cloudflare's proxy on for created records (needs SSL mode Full strict)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress set [flags]\n\nConfigures the primary domain and ACME settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if publicHTTPSPort < 0 || publicHTTPSPort > 65535 {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("--public-https-port must be between 0 and 65535"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	// PUT /api/v1/settings/ingress replaces the whole resource, so a flag
	// left at its zero value would silently clear it. Start from what's
	// already stored and apply only the flags actually given.
	req, err := client.GetIngressSettings(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get current ingress settings: %w", err))
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "primary-domain":
			req.PrimaryDomain = primaryDomain
		case "acme-enabled":
			req.ACMEEnabled = acmeEnabled
		case "acme-email":
			req.ACMEEmail = acmeEmail
		case "acme-directory-url":
			req.ACMEDirectoryURL = acmeDirectoryURL
		case "hsts-enabled":
			req.HSTSEnabled = hstsEnabled
		case "fallback-domains":
			req.FallbackDomainsEnabled = fallbackDomains
		case "public-https-port":
			req.PublicHTTPSPort = publicHTTPSPort
		case "tls-terminated-upstream":
			req.TLSTerminatedUpstream = tlsUpstream
		case "apps-base-domain":
			req.AppsBaseDomain = appsBaseDomain
		case "dns-cname-target":
			req.DNSCNAMETarget = dnsCNAMETarget
		case "dns-ttl":
			req.DNSTTLSeconds = dnsTTL
		case "dns-proxied":
			req.DNSProxied = dnsProxied
		}
	})

	settings, err := client.UpdateIngressSettings(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ingress settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printIngressSettingsHuman(stdout, settings) })
}

func printIngressSettingsHuman(out io.Writer, s ingressSettingsResource) {
	_, _ = fmt.Fprintf(out, "primary_domain:     %s\n", s.PrimaryDomain)
	_, _ = fmt.Fprintf(out, "acme_enabled:       %v\n", s.ACMEEnabled)
	_, _ = fmt.Fprintf(out, "acme_email:         %s\n", s.ACMEEmail)
	_, _ = fmt.Fprintf(out, "acme_directory_url: %s\n", s.ACMEDirectoryURL)
	_, _ = fmt.Fprintf(out, "hsts_enabled:       %v\n", s.HSTSEnabled)
	_, _ = fmt.Fprintf(out, "fallback_domains:   %v\n", s.FallbackDomainsEnabled)
	_, _ = fmt.Fprintf(out, "public_https_port:  %d\n", s.PublicHTTPSPort)
	_, _ = fmt.Fprintf(out, "tls_terminated_upstream: %v\n", s.TLSTerminatedUpstream)
	if s.AppsBaseDomain != "" {
		_, _ = fmt.Fprintf(out, "apps_base_domain:   %s\n", s.AppsBaseDomain)
	}
	if s.DNSCNAMETarget != "" {
		_, _ = fmt.Fprintf(out, "dns_cname_target:   %s\n", s.DNSCNAMETarget)
	}
	if s.DNSTTLSeconds != 0 || s.DNSProxied {
		_, _ = fmt.Fprintf(out, "dns_ttl_seconds:    %d (proxied: %v)\n", s.DNSTTLSeconds, s.DNSProxied)
	}
	if s.PublicHost != "" {
		_, _ = fmt.Fprintf(out, "public_host:        %s (%s)\n", s.PublicHost, s.PublicHostSource)
	} else {
		_, _ = fmt.Fprintf(out, "public_host:        (none, %s)\n", s.PublicHostSource)
	}
}

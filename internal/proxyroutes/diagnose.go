package proxyroutes

import (
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Diagnosis codes, stable for the API, CLI and dashboard.
const (
	DiagNone            = ""
	DiagCertExpired     = "cert_expired"
	DiagOriginCert      = "origin_cert"
	DiagPrivateCA       = "private_ca"
	DiagNoTLS           = "router_no_tls"
	DiagNoResolver      = "no_cert_resolver"
	DiagDNSMismatch     = "dns_mismatch"
	DiagPortUnreachable = "port_unreachable"
	DiagACMEPending     = "acme_pending"
	DiagACMEStuck       = "acme_stuck"
	DiagACMEFailed      = "acme_failed"
	DiagACMERateLimited = "acme_rate_limited"
)

// Diagnosis explains why a reachable route has no usable certificate.
type Diagnosis struct {
	Code     string
	Summary  string
	Hint     string
	Snippet  string
	Evidence []string
}

// DNSFacts is what the control plane resolver says about the domain.
type DNSFacts struct {
	IPs []string
	// ServerIPs are the addresses of the host the probe dials; empty when unknown.
	ServerIPs []string
	Err       string
}

// DiagnoseInput is every fact Diagnose needs, collected by the caller so the
// decision stays a pure function.
type DiagnoseInput struct {
	Domain    string
	Class     string
	Issuer    string
	NotAfter  time.Time
	Now       time.Time
	FirstSeen time.Time
	// Budget is how long a missing certificate is treated as "still being requested".
	Budget     time.Duration
	RouterName string
	// Router is nil when Traefik's API could not be read.
	Router *RouterTLS
	// Resolvers are the certResolver names other routers use.
	Resolvers          []string
	ConfiguredResolver string
	DNS                *DNSFacts
	// Port80Open is nil when not checked.
	Port80Open *bool
	// ACMELog is the proxy's own failure line for this domain, already masked.
	ACMELog string
}

// cloudflareRanges are the published proxy ranges; a record resolving into
// them is proxied and Let's Encrypt HTTP-01 may not reach the origin.
var cloudflareRanges = mustPrefixes(
	"103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "104.16.0.0/13", "108.162.192.0/18",
	"131.0.72.0/22", "141.101.64.0/18", "162.158.0.0/15", "172.64.0.0/13", "173.245.48.0/20",
	"188.114.96.0/20", "190.93.240.0/20", "197.234.240.0/22", "198.41.128.0/17",
)

func mustPrefixes(s ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(s))
	for _, p := range s {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}

// ProxiedByCDN reports whether any address sits in a CDN proxy range.
func ProxiedByCDN(ips []string) bool {
	for _, s := range ips {
		a, err := netip.ParseAddr(s)
		if err != nil {
			continue
		}
		for _, p := range cloudflareRanges {
			if p.Contains(a) {
				return true
			}
		}
	}
	return false
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// Diagnose returns the most likely cause, or a zero Diagnosis when the
// certificate is fine. For a fallback certificate the order is: router has no
// resolver, DNS points elsewhere, port 80 closed, then ACME pending or stuck.
func Diagnose(in DiagnoseInput) Diagnosis {
	switch {
	case in.Class == ClassTrusted && !in.NotAfter.IsZero() && in.Now.After(in.NotAfter):
		return Diagnosis{Code: DiagCertExpired, Summary: "the certificate has expired",
			Hint:     "The proxy did not renew it. Check the proxy logs for ACME errors and that port 80 or the DNS challenge still works.",
			Evidence: []string{fmt.Sprintf("issuer %s, expired %s", in.Issuer, in.NotAfter.UTC().Format("2006-01-02"))}}
	case in.Class == ClassTrusted || in.Class == "":
		return Diagnosis{}
	case in.Class == ClassOriginCA:
		return Diagnosis{Code: DiagOriginCert, Summary: "the proxy serves an origin certificate (" + in.Issuer + ")",
			Hint:     "Browsers only trust this certificate when a CDN sits in front. Proxy the DNS record through the CDN, or give the proxy a public certificate.",
			Evidence: []string{"issuer " + in.Issuer}}
	case in.Class == ClassPrivateCA:
		return Diagnosis{Code: DiagPrivateCA, Summary: "the proxy serves a certificate from a private authority (" + in.Issuer + ")",
			Hint:     "Public clients will not trust it. Use a public certificate resolver for this domain, or trust the authority on every client.",
			Evidence: []string{"issuer " + in.Issuer}}
	}
	return diagnoseFallback(in)
}

func diagnoseFallback(in DiagnoseInput) Diagnosis {
	ev := []string{"the proxy answered with its fallback certificate: " + in.Issuer}
	router := in.RouterName
	if in.Router != nil && in.Router.Found {
		switch {
		case !in.Router.HasTLS:
			ev = append(ev, "router "+router+" has no tls block")
			return Diagnosis{Code: DiagNoTLS, Summary: "router " + router + " does not enable TLS",
				Hint: "Enable TLS on the router and give it a certificate resolver.", Evidence: ev,
				Snippet: resolverSnippet(router, pickResolver(in), true)}
		case in.Router.CertResolver == "":
			ev = append(ev, "router "+router+" has TLS but no certResolver")
			if len(in.Resolvers) > 0 {
				ev = append(ev, "resolvers in use on this proxy: "+strings.Join(in.Resolvers, ", "))
			}
			res := pickResolver(in)
			hint := "Router " + router + " has TLS but no certResolver, so the proxy never asks for a certificate. Add the resolver"
			if res == "" {
				hint += " (define one in the proxy static config first)."
			} else {
				hint += " " + res + " in the proxy integration settings, or as the label below."
			}
			return Diagnosis{Code: DiagNoResolver, Summary: "router " + router + " has TLS but no certResolver",
				Hint: hint, Evidence: ev, Snippet: resolverSnippet(router, res, false)}
		}
		ev = append(ev, "router "+router+" uses certResolver "+in.Router.CertResolver)
	}
	if d := dnsDiagnosis(in, ev); d != nil {
		return *d
	}
	if in.DNS != nil && len(in.DNS.IPs) > 0 {
		ev = append(ev, in.Domain+" resolves to "+strings.Join(in.DNS.IPs, ", "))
	}
	if in.Port80Open != nil && !*in.Port80Open {
		ev = append(ev, "port 80 did not accept a connection on this server")
		return Diagnosis{Code: DiagPortUnreachable, Summary: "port 80 is not reachable, so the HTTP-01 challenge cannot pass",
			Hint: "Open ports 80 and 443 to the internet on the firewall and cloud security group, and make sure the proxy publishes port 80. Or use the DNS challenge.", Evidence: ev}
	}
	if in.ACMELog != "" {
		ev = append(ev, "proxy log: "+in.ACMELog)
		if IsACMERateLimit(in.ACMELog) {
			return Diagnosis{Code: DiagACMERateLimited, Summary: "the certificate authority is rate limiting this domain",
				Hint: "Wait for the limit to lift (often up to a week for repeated failures), fix the underlying error first, or use the staging authority while testing.", Evidence: ev}
		}
		return Diagnosis{Code: DiagACMEFailed, Summary: "the proxy tried to get a certificate and failed",
			Hint: "Fix the cause in the proxy log line below, then use Retry certificate request. A failed attempt is not repeated until the proxy reloads its configuration.", Evidence: ev}
	}
	waited := in.Now.Sub(in.FirstSeen)
	if in.FirstSeen.IsZero() || waited < in.Budget {
		return Diagnosis{Code: DiagACMEPending, Summary: "the proxy is still requesting a certificate",
			Hint: "A first certificate usually takes under a minute. This page rechecks on its own.", Evidence: ev}
	}
	ev = append(ev, fmt.Sprintf("no certificate after %s", waited.Round(time.Second)))
	return Diagnosis{Code: DiagACMEStuck, Summary: "the proxy still has no certificate for this domain",
		Hint: "Read the proxy logs for the ACME error (rate limit, failed challenge, wrong email or resolver). For Traefik: docker logs <traefik container> 2>&1 | grep -i acme", Evidence: ev}
}

func dnsDiagnosis(in DiagnoseInput, ev []string) *Diagnosis {
	if in.DNS == nil {
		return nil
	}
	if in.DNS.Err != "" {
		ev = append(ev, "DNS lookup failed: "+in.DNS.Err)
		return &Diagnosis{Code: DiagDNSMismatch, Summary: "the domain does not resolve",
			Hint: "Create an A or AAAA record for the domain that points to the server running the proxy.", Evidence: ev}
	}
	if len(in.DNS.ServerIPs) == 0 || len(in.DNS.IPs) == 0 || overlap(in.DNS.IPs, in.DNS.ServerIPs) {
		return nil
	}
	ev = append(ev, in.Domain+" resolves to "+strings.Join(in.DNS.IPs, ", "), "this server is "+strings.Join(in.DNS.ServerIPs, ", "))
	hint := "Point the A or AAAA record at the server running the proxy."
	if ProxiedByCDN(in.DNS.IPs) {
		ev = append(ev, "the record looks proxied by a CDN")
		hint = "The record is proxied by a CDN, so the certificate challenge cannot reach this server. Set it to DNS only until the certificate is issued, or use the DNS challenge."
	}
	return &Diagnosis{Code: DiagDNSMismatch, Summary: "the domain does not point to this server", Hint: hint, Evidence: ev}
}

func pickResolver(in DiagnoseInput) string {
	if in.ConfiguredResolver != "" {
		return in.ConfiguredResolver
	}
	if len(in.Resolvers) > 0 {
		return in.Resolvers[0]
	}
	return ""
}

func resolverSnippet(router, resolver string, withTLS bool) string {
	if resolver == "" {
		resolver = "<resolver>"
	}
	var b strings.Builder
	if withTLS {
		fmt.Fprintf(&b, "traefik.http.routers.%s.tls=true\n", router)
	}
	fmt.Fprintf(&b, "traefik.http.routers.%s.tls.certresolver=%s", router, resolver)
	return b.String()
}

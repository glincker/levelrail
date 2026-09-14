package ingress

import (
	"net"
	"strings"
)

// fallbackDomainSuffix is sslip.io's own wildcard DNS convention: any
// hostname ending in this suffix, with an IP address dash-encoded
// somewhere in one of its labels, resolves to that IP with zero DNS
// setup. See https://sslip.io. Chosen over nip.io/traefik.me only
// because it's the one Coolify and Dokploy both already point
// operators at, so a shared fallback host an operator has likely
// already seen before is one less thing to explain.
const fallbackDomainSuffix = "sslip.io"

// FallbackDomain synthesizes the zero-config, publicly resolvable HTTPS
// hostname a fresh deploy with no domain configured gets routed under:
// "<app>.<public-ip-dash-encoded>.sslip.io". Real DNS, so Caddy's
// existing automatic-HTTPS path (internal/reconcile/ingress's own ACME
// vs internal-issuer choice, governed by store.IngressSettings.
// ACMEEnabled exactly like any operator-configured domain) needs no
// special case to issue it a certificate, as long as this control
// plane's own :80/:443 are genuinely reachable at publicHost from the
// public internet, the same precondition any custom domain already has.
//
// ok is false, meaning "no fallback available, route nothing," when:
//   - publicHost isn't a literal IP address (APP_PUBLIC_HOST as a
//     hostname has no address to dash-encode; a hostname operator
//     already has a resolvable name and should set a real domain
//     instead)
//   - the IP is private, loopback, link-local, or unspecified: sslip.io
//     would still resolve it, but Let's Encrypt's HTTP-01 challenge
//     could never reach a non-public address regardless, so producing
//     a URL that can only ever fail to get a trusted certificate is
//     worse than producing none
//   - serviceName has no characters left after sanitizing into a valid
//     DNS label (lowercased, non-alphanumeric runs collapsed to a
//     single hyphen, leading/trailing hyphens trimmed)
func FallbackDomain(publicHost, serviceName string) (domain string, ok bool) {
	ip := net.ParseIP(strings.TrimSpace(publicHost))
	if ip == nil {
		return "", false
	}
	if !isPubliclyRoutable(ip) {
		return "", false
	}
	label := sanitizeDNSLabel(serviceName)
	if label == "" {
		return "", false
	}
	return label + "." + dashEncodeIP(ip) + "." + fallbackDomainSuffix, true
}

// isPubliclyRoutable reports whether ip is a real, internet-reachable
// address: not private (RFC 1918 / ULA), not loopback, not link-local,
// not the unspecified address.
func isPubliclyRoutable(ip net.IP) bool {
	return !ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified()
}

// dashEncodeIP renders ip the way sslip.io expects to find one embedded
// in a hostname label: dots replaced with dashes for IPv4
// ("203-0-113-5"), colons replaced with dashes for IPv6
// ("2001-db8--1"), matching sslip.io's own documented format for both
// families.
func dashEncodeIP(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return strings.ReplaceAll(v4.String(), ".", "-")
	}
	return strings.ReplaceAll(ip.String(), ":", "-")
}

// sanitizeDNSLabel lowercases s and replaces every run of characters
// that isn't a lowercase letter, digit, or hyphen with a single hyphen,
// then trims leading/trailing hyphens: the same tolerant, "make it
// valid rather than reject it" approach a generated hostname (as
// opposed to operator-typed input, which validateAppResource already
// gates) calls for.
func sanitizeDNSLabel(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

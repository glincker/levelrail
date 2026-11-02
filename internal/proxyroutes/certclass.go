package proxyroutes

import "strings"

// Certificate classes a probe can report.
const (
	ClassTrusted      = "trusted"
	ClassProxyDefault = "proxy_default"
	ClassOriginCA     = "origin_ca"
	ClassPrivateCA    = "private_ca"
)

// fallbackIssuers are the certificates proxies serve when they hold none for
// the requested host. Matching is on the lowercased issuer common name or
// organisation.
var fallbackIssuers = []string{
	"traefik default cert",
	"traefik",
	"caddy local authority",
	"kubernetes ingress controller fake certificate",
	"acme co",
}

// originIssuers are CDN origin CAs: valid for the CDN-to-origin hop only.
var originIssuers = []string{"cloudflare origin", "cloudflare, inc."}

// ClassifyCert sorts a presented certificate. A self-signed certificate for
// localhost is the nginx default; any other untrusted issuer is a private CA.
func ClassifyCert(issuerCN, issuerOrg, subjectCN string, selfSigned, trusted bool) string {
	cn, org := strings.ToLower(issuerCN), strings.ToLower(issuerOrg)
	if trusted {
		return ClassTrusted
	}
	for _, f := range fallbackIssuers {
		if cn == f || org == f {
			return ClassProxyDefault
		}
	}
	if selfSigned && strings.EqualFold(subjectCN, "localhost") {
		return ClassProxyDefault
	}
	for _, o := range originIssuers {
		if strings.Contains(cn, o) || strings.Contains(org, o) {
			return ClassOriginCA
		}
	}
	return ClassPrivateCA
}

// UntrustedMessage is the short probe error for a reachable route whose
// certificate does not verify; the full diagnosis comes from Diagnose.
func UntrustedMessage(class, issuer string) string {
	switch class {
	case ClassProxyDefault:
		return "reachable, but the proxy served its own fallback certificate (" + issuer + "), so it holds no certificate for this domain"
	case ClassOriginCA:
		return "reachable, but the certificate (" + issuer + ") is an origin certificate that browsers do not trust directly"
	default:
		return "reachable, but the certificate (issued by " + issuer + ") is not trusted by this server"
	}
}

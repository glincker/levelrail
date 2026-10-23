package domaindoctor

import "strings"

// ACMEActionFixDNSAccess is reported when the DNS provider refused the
// _acme-challenge record the platform tried to create.
const ACMEActionFixDNSAccess = "fix_dns_access"

type acmeKind int

const (
	acmeOther acmeKind = iota
	acmeRateLimited
	acmeCAA
	acmeDNSPermission
	acmeNoRecords
	acmeOtherProxy
	acmeChallengeFailed
)

func classifyACME(raw string) acmeKind {
	l := strings.ToLower(raw)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(l, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("ratelimited", "rate limit", "too many certificates", "too many failed"):
		return acmeRateLimited
	case has("caa"):
		return acmeCAA
	case has("dns-01", "_acme-challenge", "txt record") && has("403", "401", "unauthorized", "forbidden", "authentication", "permission", "accessdenied", "access denied", "invalid api token", "not authorized"):
		return acmeDNSPermission
	case has("nxdomain", "no valid a records", "no valid ip addresses", "dns problem"):
		return acmeNoRecords
	case has("invalid response from http://", "acme:error:tls", "unrecognized name"):
		return acmeOtherProxy
	case has("timeout", "connection refused", "no route", "firewall", "unreachable", "challenge failed", "invalid response", "authorization"):
		return acmeChallengeFailed
	}
	return acmeOther
}

// ExplainACME turns the certificate authority's raw error into one plain
// sentence an operator can act on.
func ExplainACME(raw string) string {
	switch classifyACME(raw) {
	case acmeRateLimited:
		return "Let's Encrypt is rate limiting this domain after too many certificates or failed attempts. The limit lifts by itself, within a week at most; the platform retries on its own."
	case acmeCAA:
		return "A CAA record on this domain does not allow Let's Encrypt to issue certificates."
	case acmeDNSPermission:
		return "The DNS provider refused the platform's token when it tried to create the _acme-challenge TXT record. The token needs permission to edit DNS for this zone."
	case acmeNoRecords:
		return "Let's Encrypt could not find DNS records for this domain, so it could not check that you own it."
	case acmeOtherProxy:
		return "Something other than this platform answered Let's Encrypt's check on port 80 or 443, usually another proxy in front."
	case acmeChallengeFailed:
		return "Let's Encrypt could not reach this server on port 80 to check that you own the domain (the challenge failed)."
	}
	return "Let's Encrypt refused to issue a certificate. The CA's own message is below."
}

// effectiveACMEAction prefers the specific action the API classified, and
// refines the generic one from the raw error.
func effectiveACMEAction(f *ACMEFailureFacts) string {
	if classifyACME(f.Error) == acmeDNSPermission {
		return ACMEActionFixDNSAccess
	}
	return f.Action
}

func explainedDetail(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 300 {
		raw = raw[:300] + "..."
	}
	return ExplainACME(raw) + " CA said: " + raw
}

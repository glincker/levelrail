package domaindoctor

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsinspect"
)

// DNS verdicts, computed from authoritative answers first.
const (
	VerdictReady       = "dns_ready"
	VerdictPublicReady = "public_ready"
	VerdictNotFound    = "not_found"
	VerdictElsewhere   = "points_elsewhere"
	VerdictUnknown     = "unknown"
)

// Hint codes; the dashboard translates them, Message is the English text.
const (
	HintLocalStale      = "local_stale"
	HintWasCloudflare   = "was_cloudflare_proxy"
	HintCloudflareNow   = "cloudflare_proxied"
	HintWaiting         = "waiting"
	HintAAAAElsewhere   = "aaaa_elsewhere"
	HintNSMismatch      = "ns_mismatch"
	HintCheckZone       = "check_zone"
	HintCheckAAAA       = "check_aaaa"
	HintCheckCNAME      = "check_cname"
	HintCheckNameserver = "check_nameservers"
	HintLocalStaleLong  = "local_stale_long"
)

// DNSHint is one piece of guidance.
type DNSHint struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DNSVerdictInput is what JudgeDNS needs.
type DNSVerdictInput struct {
	// Expected are this server's public addresses.
	Expected   []string
	Inspection dnsinspect.Inspection
	Local      dnsinspect.Source
	Now        time.Time
	// Waiting is how long this domain has been unsettled; Budget is when the
	// card stops reassuring and lists concrete checks.
	Waiting time.Duration
	Budget  time.Duration
	// FallbackWait is the cache wait assumed when the local TTL is unknown.
	FallbackWait time.Duration
	FlushCommand string
}

// DNSVerdict is the judged state of a domain's DNS.
type DNSVerdict struct {
	Verdict      string
	LocalStale   bool
	Escalated    bool
	RetryAt      time.Time
	FlushCommand string
	Hints        []DNSHint
}

func sourceAddrs(srcs []dnsinspect.Source) []string {
	var out []string
	for _, s := range srcs {
		for _, a := range s.Addresses {
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return out
}

// JudgeDNS decides readiness from authoritative answers, so a stale cache on
// this host can never hold a correctly configured domain back.
func JudgeDNS(in DNSVerdictInput) DNSVerdict {
	auth, pub := sourceAddrs(in.Inspection.Authoritative), sourceAddrs(in.Inspection.Public)
	v := DNSVerdict{Verdict: VerdictUnknown}
	switch {
	case len(auth) > 0:
		v.Verdict = VerdictElsewhere
		if overlaps(auth, in.Expected) {
			v.Verdict = VerdictReady
		}
	case in.Inspection.AuthReachable:
		v.Verdict = VerdictNotFound
	case len(pub) > 0:
		v.Verdict = VerdictElsewhere
		if overlaps(pub, in.Expected) {
			v.Verdict = VerdictPublicReady
		}
	case len(in.Inspection.Public) > 0:
		v.Verdict = VerdictNotFound
	}
	ready := v.Verdict == VerdictReady || v.Verdict == VerdictPublicReady
	v.LocalStale = ready && !overlaps(in.Local.Addresses, in.Expected)
	v.FlushCommand = in.FlushCommand
	v.Escalated = in.Budget > 0 && in.Waiting >= in.Budget && (!ready || v.LocalStale) && v.Verdict != VerdictUnknown

	if v.LocalStale {
		wait := time.Duration(in.Local.TTL) * time.Second
		if wait <= 0 {
			wait = in.FallbackWait
		}
		v.RetryAt = in.Now.Add(wait).UTC()
		v.Hints = append(v.Hints, localStaleHint(v.RetryAt, v.Escalated))
	}
	if ready && allCloudflare(in.Local.Addresses) && !allCloudflare(sourceAddrs(in.Inspection.Authoritative)) {
		v.Hints = append(v.Hints, DNSHint{HintWasCloudflare, "This domain was proxied by Cloudflare; your DNS now points straight at the server, so visitors reach the server directly (the origin IP is no longer hidden)."})
	}
	if v.Verdict == VerdictElsewhere && allCloudflare(auth) {
		v.Hints = append(v.Hints, DNSHint{HintCloudflareNow, "The record is still proxied by Cloudflare (orange cloud). Switch it to DNS only (grey cloud) or point it at " + firstOr(in.Expected, "this server") + "."})
	}
	if v6 := ipv6Only(auth); len(v6) > 0 && !overlaps(v6, ipv6Only(in.Expected)) && v.Verdict != VerdictUnknown {
		v.Hints = append(v.Hints, DNSHint{HintAAAAElsewhere, "An AAAA record points IPv6 visitors at " + strings.Join(v6, ", ") + ", not this server. Remove it or point it here."})
	}
	if a, b := in.Inspection.DelegatedNS, in.Inspection.ZoneNS; len(a) > 0 && len(b) > 0 && !slices.Equal(a, b) {
		v.Hints = append(v.Hints, DNSHint{HintNSMismatch, fmt.Sprintf("The registrar delegates %s to %s but the zone itself lists %s. Another DNS provider may be authoritative.", in.Inspection.Zone, strings.Join(a, ", "), strings.Join(b, ", "))})
	}
	if v.Escalated && !ready {
		v.Hints = append(v.Hints, escalationHints(in)...)
	} else if !ready && v.Verdict != VerdictUnknown {
		v.Hints = append(v.Hints, DNSHint{HintWaiting, "The record has not appeared on the authoritative servers yet. New records usually show within a few minutes."})
	}
	return v
}

func localStaleHint(retry time.Time, escalated bool) DNSHint {
	if escalated {
		return DNSHint{HintLocalStaleLong, "This server's own resolver still holds the old answer, longer than expected. DNS is correct, so certificates and visitors are not affected; flush the cache below or restart the resolver."}
	}
	return DNSHint{HintLocalStale, fmt.Sprintf("DNS is correct at the authoritative servers. This server's own resolver still holds the old answer and will refresh at %s UTC; nothing else is blocked.", retry.Format("15:04"))}
}

func escalationHints(in DNSVerdictInput) []DNSHint {
	zone := firstOr([]string{in.Inspection.Zone}, "the domain")
	return []DNSHint{
		{HintCheckZone, "Check the record is in the right DNS zone (" + zone + ") and that its name is spelled exactly."},
		{HintCheckAAAA, "Check there is no AAAA record pointing somewhere else."},
		{HintCheckCNAME, "Check the apex is not a CNAME that your provider flattens to a different address."},
		{HintCheckNameserver, "Check another DNS provider is not authoritative: the name servers at your registrar must be the ones holding this record."},
	}
}

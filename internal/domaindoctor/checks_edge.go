package domaindoctor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/netip"
	"slices"
	"strings"
)

// IDs for the checks that explain what sits between visitors and this server.
const (
	IDDNSProxied  = "dns.proxied"
	IDDNSWildcard = "dns.wildcard"
)

// cloudflareRanges are Cloudflare's published proxy ranges (cloudflare.com/ips).
var cloudflareRanges = mustPrefixes(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18",
	"108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17",
	"162.158.0.0/15", "104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
	"2a06:98c0::/29", "2c0f:f248::/32",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// IsCloudflareIP reports whether ip is inside Cloudflare's proxy ranges.
func IsCloudflareIP(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(cloudflareRanges, func(p netip.Prefix) bool { return p.Contains(a.Unmap()) })
}

func allCloudflare(ips []string) bool {
	return len(ips) > 0 && !slices.ContainsFunc(ips, func(ip string) bool { return !IsCloudflareIP(ip) })
}

func isCloudflareTarget(target string) bool {
	t := strings.ToLower(strings.TrimSuffix(target, "."))
	return strings.HasSuffix(t, ".cdn.cloudflare.net") || strings.HasSuffix(t, ".cloudflare.net")
}

// proxiedByCloudflare is true when every resolved address is Cloudflare's or
// a CNAME points at Cloudflare, and none of them is this server.
func (rn *run) proxiedByCloudflare(ctx context.Context) bool {
	if len(rn.resolved) == 0 || overlaps(rn.resolved, rn.facts.ServerIPs) {
		return false
	}
	if allCloudflare(rn.resolved) {
		return true
	}
	if rn.probes.DNS == nil || IsApex(rn.facts.Domain) {
		return false
	}
	pctx, cancel := rn.probeCtx(ctx)
	defer cancel()
	target, err := rn.probes.DNS.CNAME(pctx, rn.facts.Domain)
	return err == nil && isCloudflareTarget(target)
}

func (rn *run) checkProxied(ctx context.Context) Check {
	c := Check{ID: IDDNSProxied, Title: "Not hidden behind a proxy", Tier: TierCertificate}
	if len(rn.resolved) == 0 {
		c.State, c.Detail = StateSkipped, "waiting on "+IDDNSResolves
		return c
	}
	if !rn.proxiedByCloudflare(ctx) {
		c.State = StatePass
		return c
	}
	c.State = StateWarn
	c.Title = rn.facts.Domain + " is behind Cloudflare's proxy (orange cloud)"
	c.Detail = "the addresses visitors get belong to Cloudflare, so this server's own address cannot be compared and the doctor did not connect to it"
	c.Fix = &Fix{Summary: "In Cloudflare, point the record at " + firstOr(rn.facts.ServerIPs, "this server's address") +
		", set SSL/TLS to Full (strict) so visitors are not sent in circles, or switch the record to DNS only (grey cloud) so Let's Encrypt validates this server directly."}
	return c
}

func randomLabel() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return "probe-" + hex.EncodeToString(b)
}

// checkWildcard looks for a wildcard record answering for a name that was
// never created; for a wildcard domain it checks the DNS-01 requirement.
func (rn *run) checkWildcard(ctx context.Context) Check {
	c := Check{ID: IDDNSWildcard, Title: "Wildcard DNS is understood", Tier: TierHygiene}
	if base, ok := strings.CutPrefix(rn.facts.Domain, "*."); ok {
		return rn.checkWildcardDomain(c, base)
	}
	if len(rn.resolved) == 0 || rn.probes.DNS == nil {
		c.State, c.Detail = StateSkipped, "waiting on "+IDDNSResolves
		return c
	}
	_, parent, ok := strings.Cut(rn.facts.Domain, ".")
	if !ok || IsApex(rn.facts.Domain) {
		parent = rn.facts.Domain
	}
	pctx, cancel := context.WithTimeout(ctx, 2*rn.opts.ProbeTimeout)
	defer cancel()
	var addrs []string
	for _, a := range rn.probes.DNS.Answers(pctx, randomLabel()+"."+parent) {
		addrs = append(addrs, a.IPv4...)
		addrs = append(addrs, a.IPv6...)
	}
	if len(addrs) == 0 {
		c.State, c.Detail = StatePass, "no wildcard record under "+parent
		return c
	}
	c.State = StatePass
	if overlaps(addrs, rn.resolved) {
		c.Detail = "a wildcard record under " + parent + " answers for this name too; a typo in any subdomain resolves here as well"
	} else {
		c.Detail = "a wildcard record under " + parent + " exists, but " + rn.facts.Domain + " has its own record"
	}
	return c
}

func (rn *run) checkWildcardDomain(c Check, base string) Check {
	c.Title = "Wildcard certificates need a DNS provider"
	switch {
	case rn.facts.TLSTerminatedUpstream:
		c.State, c.Detail = StateSkipped, "TLS is terminated by your own proxy"
	case !rn.facts.ACMEEnabled:
		c.State, c.Detail = StateSkipped, "trusted certificates are off on this platform"
	case rn.facts.DNSProvider == "":
		c.State = StateWarn
		c.Detail = "Let's Encrypt only issues *." + base + " over DNS-01, which needs a connected Cloudflare or Route 53 account"
		c.Fix = &Fix{Summary: "Connect a DNS provider so the platform can answer the DNS-01 challenge.",
			Action: &Action{Kind: ActionLink, Label: "Connect DNS provider", API: "/domains"}}
	default:
		c.State = StatePass
		c.Detail = "DNS-01 through " + providerLabel(rn.facts.DNSProvider)
	}
	return c
}

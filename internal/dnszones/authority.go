package dnszones

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"
)

// Authority provider labels. Cloudflare and Route53 reuse the Provider constants.
const (
	AuthorityGoogle   = "google"
	AuthorityOther    = "other"
	AuthorityThisHost = "this_host"
)

// Plain-language risks reported by CheckAuthority.
const (
	RiskNSOnThisHost = "name servers run on this host, DNS stops with it"
	RiskProxied      = "Cloudflare proxy shows its own error page if the origin is down"
	RiskTTLHigh      = "TTL above the warning threshold slows failover"
	RiskNoNS         = "no NS records found"
)

// ErrAuthorityLookup wraps resolver failures so callers can tell them apart from empty answers.
var ErrAuthorityLookup = errors.New("dnszones: authority lookup failed")

// cloudflareProxyRanges is Cloudflare's published proxy list (cloudflare.com/ips),
// copied here so the check needs no network call to classify an answer.
var cloudflareProxyRanges = mustPrefixes(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// IsCloudflareProxyAddr reports whether addr is inside a Cloudflare proxy range.
func IsCloudflareProxyAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range cloudflareProxyRanges {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// AuthorityResolver is the lookup surface the check needs, so tests run offline.
type AuthorityResolver interface {
	LookupNS(ctx context.Context, name string) ([]string, error)
	LookupIP(ctx context.Context, name string) ([]netip.Addr, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
}

// TTLResolver is optional: a resolver that also exposes the answer TTL implements it.
type TTLResolver interface {
	LookupTTL(ctx context.Context, name string) (time.Duration, bool, error)
}

// AuthorityOptions configures CheckAuthority. The runtime-settings wiring for
// WarnTTL comes with the API slice; the caller supplies the value.
type AuthorityOptions struct {
	Resolver AuthorityResolver
	// LocalIPs are this control plane's own addresses.
	LocalIPs []netip.Addr
	// WarnTTL flags slower failover when the observed TTL exceeds it. Zero disables the check.
	WarnTTL time.Duration
}

// NameServer is one NS name with the provider it maps to.
type NameServer struct {
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	IPs      []string `json:"ips,omitempty"`
}

// Authority is who answers for a domain right now and what that means for failover.
type Authority struct {
	Domain      string       `json:"domain"`
	NameServers []NameServer `json:"name_servers"`
	Providers   []string     `json:"providers"`
	Answers     []string     `json:"answers,omitempty"`
	CNAME       string       `json:"cname,omitempty"`
	Proxied     bool         `json:"proxied"`
	PointsHere  bool         `json:"points_here"`
	// TTL is nil when the resolver does not expose it.
	TTL   *time.Duration `json:"ttl,omitempty"`
	Risks []string       `json:"risks"`
}

// CheckAuthority resolves NS and address answers for domain and derives the risks.
func CheckAuthority(ctx context.Context, domain string, opts AuthorityOptions) (Authority, error) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" {
		return Authority{}, errors.New("dnszones: domain is required")
	}
	if opts.Resolver == nil {
		return Authority{}, errors.New("dnszones: authority resolver is required")
	}
	res := Authority{Domain: domain, Risks: []string{}, Providers: []string{}}

	nsNames, err := opts.Resolver.LookupNS(ctx, domain)
	if err != nil {
		return Authority{}, fmt.Errorf("%w: NS for %s: %w", ErrAuthorityLookup, domain, err)
	}
	local := make(map[netip.Addr]bool, len(opts.LocalIPs))
	for _, ip := range opts.LocalIPs {
		local[ip.Unmap()] = true
	}
	seen := map[string]bool{}
	for _, raw := range nsNames {
		name := strings.TrimSuffix(strings.ToLower(raw), ".")
		if name == "" {
			continue
		}
		ns := NameServer{Name: name, Provider: classifyNS(name)}
		// An NS host that fails to resolve is still reported, just not checked for locality.
		if ips, ipErr := opts.Resolver.LookupIP(ctx, name); ipErr == nil {
			for _, ip := range ips {
				ns.IPs = append(ns.IPs, ip.Unmap().String())
				if local[ip.Unmap()] {
					ns.Provider = AuthorityThisHost
				}
			}
		}
		res.NameServers = append(res.NameServers, ns)
		if !seen[ns.Provider] {
			seen[ns.Provider] = true
			res.Providers = append(res.Providers, ns.Provider)
		}
	}
	sort.Strings(res.Providers)
	if res.NameServers == nil {
		res.NameServers = []NameServer{}
	}

	ips, err := opts.Resolver.LookupIP(ctx, domain)
	if err != nil {
		return Authority{}, fmt.Errorf("%w: address for %s: %w", ErrAuthorityLookup, domain, err)
	}
	for _, ip := range ips {
		ip = ip.Unmap()
		res.Answers = append(res.Answers, ip.String())
		if IsCloudflareProxyAddr(ip) {
			res.Proxied = true
		}
		if local[ip] {
			res.PointsHere = true
		}
	}
	// A CNAME is informational only, so a failure here does not fail the check.
	if cname, cErr := opts.Resolver.LookupCNAME(ctx, domain); cErr == nil {
		if c := strings.TrimSuffix(strings.ToLower(cname), "."); c != domain {
			res.CNAME = c
		}
	}
	if tr, ok := opts.Resolver.(TTLResolver); ok {
		if ttl, known, tErr := tr.LookupTTL(ctx, domain); tErr == nil && known {
			res.TTL = &ttl
		}
	}

	if len(res.NameServers) == 0 {
		res.Risks = append(res.Risks, RiskNoNS)
	}
	if seen[AuthorityThisHost] {
		res.Risks = append(res.Risks, RiskNSOnThisHost)
	}
	if res.Proxied {
		res.Risks = append(res.Risks, RiskProxied)
	}
	if res.TTL != nil && opts.WarnTTL > 0 && *res.TTL > opts.WarnTTL {
		res.Risks = append(res.Risks, RiskTTLHigh)
	}
	return res, nil
}

func classifyNS(name string) string {
	switch {
	case strings.HasSuffix(name, ".ns.cloudflare.com"):
		return ProviderCloudflare
	case strings.Contains(name, "awsdns"):
		return ProviderRoute53
	case strings.Contains(name, "googledomains") || strings.HasPrefix(name, "ns-cloud"):
		return AuthorityGoogle
	default:
		return AuthorityOther
	}
}

package email

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
)

// Diagnosis codes added on top of the base SPF, DKIM and DMARC codes.
const (
	CodeSPFTooManyLookups = "spf_too_many_lookups"
	CodeDKIMProxied       = "dkim_cname_proxied"
	CodeNotAligned        = "from_not_aligned"
	CodeAligned           = "from_aligned"

	RecordAlignment = "alignment"
)

// SPFLimits bounds the lookup count and the include nesting walked by CountSPFLookups.
type SPFLimits struct {
	Lookups int
	Depth   int
}

// SPFLimitsFromEnv reads APP_SPF_LOOKUP_LIMIT and APP_SPF_DEPTH_LIMIT. The defaults are the protocol limit of 10.
func SPFLimitsFromEnv() SPFLimits {
	return SPFLimits{Lookups: envPositive("APP_SPF_LOOKUP_LIMIT", 10), Depth: envPositive("APP_SPF_DEPTH_LIMIT", 10)}
}

func envPositive(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

// CountSPFLookups counts the DNS-querying terms of an SPF record and of every record it includes or redirects to.
// It stops descending at maxDepth and reports the running total, so a loop or a deep chain cannot run away.
func CountSPFLookups(ctx context.Context, dns DNSLookup, record string, maxDepth int) int {
	return countSPF(ctx, dns, record, maxDepth, map[string]bool{})
}

func countSPF(ctx context.Context, dns DNSLookup, record string, depth int, seen map[string]bool) int {
	total := 0
	for _, term := range strings.Fields(strings.ToLower(record)) {
		term = strings.TrimLeft(term, "+-~?")
		name, target := term, ""
		if i := strings.IndexAny(term, ":=/"); i >= 0 {
			name = term[:i]
			if term[i] != '/' {
				target = term[i+1:]
			}
		}
		switch name {
		case "a", "mx", "ptr", "exists":
			total++
		case "include", "redirect":
			total++
			if target == "" || depth <= 0 || seen[target] {
				continue
			}
			seen[target] = true
			total += countSPF(ctx, dns, firstSPF(ctx, dns, target), depth-1, seen)
		}
	}
	return total
}

func firstSPF(ctx context.Context, dns DNSLookup, name string) string {
	txts, _ := dns.LookupTXT(ctx, name)
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			return t
		}
	}
	return ""
}

// HostLookup is the optional address lookup some resolvers offer; the proxy check needs it.
type HostLookup interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

var cloudflareRanges = parseCIDRs(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18",
	"108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17",
	"162.158.0.0/15", "104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
	"2a06:98c0::/29", "2c0f:f248::/32",
)

func parseCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func isCloudflareIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	for _, n := range cloudflareRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// DKIMProxied reports whether name answers with Cloudflare addresses instead of the CNAME a DKIM
// delegation needs. It returns false when the resolver cannot look up addresses, so it never guesses.
func DKIMProxied(ctx context.Context, dns DNSLookup, name string) bool {
	h, ok := dns.(HostLookup)
	if !ok {
		return false
	}
	if txts, err := dns.LookupTXT(ctx, name); err == nil && len(txts) > 0 {
		return false
	}
	if c, err := dns.LookupCNAME(ctx, name); err == nil && strings.TrimSuffix(strings.ToLower(c), ".") != strings.ToLower(name) && c != "" {
		return false
	}
	addrs, err := h.LookupHost(ctx, name)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		if !isCloudflareIP(a) {
			return false
		}
	}
	return true
}

func proxiedDKIMCheck(name string) RecordCheck {
	return RecordCheck{
		Type: RecordDKIM, Name: name, Status: CheckWarn, Code: CodeDKIMProxied,
		Message: "The DKIM record at " + name + " is proxied through Cloudflare, so receivers get Cloudflare addresses instead of the CNAME. Switch the record to DNS only (grey cloud).",
	}
}

// CheckAlignment grades DMARC alignment of the From domain against the SPF (return-path) and DKIM (d=) domains.
// Alignment is relaxed: the domains match, or one is a subdomain of the other.
func CheckAlignment(from, spfDomain, dkimDomain string) RecordCheck {
	from = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(from), "."))
	rc := RecordCheck{Type: RecordAlignment, Name: from}
	for _, d := range []string{spfDomain, dkimDomain} {
		if domainsAlign(from, d) {
			rc.Status, rc.Code, rc.Message = CheckPass, CodeAligned, "The From domain "+from+" is aligned with "+strings.ToLower(strings.TrimSpace(d))+"."
			return rc
		}
	}
	rc.Status, rc.Code = CheckWarn, CodeNotAligned
	rc.Message = "The From domain " + from + " matches neither the SPF domain nor the DKIM domain, so DMARC fails even when both pass. Send from a domain your provider signs for, or add a custom return-path domain."
	return rc
}

func domainsAlign(a, b string) bool {
	a = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(a), "."))
	b = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(b), "."))
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

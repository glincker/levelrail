package dnsinspect

import "strings"

// Name server provider classes reported by ProviderOf.
const (
	ProviderCloudflare = "cloudflare"
	ProviderRoute53    = "route53"
	ProviderOther      = "other"
	ProviderUnknown    = "unknown"
)

// ProviderOf classifies a zone by its name server hostnames so the UI can
// offer to connect the matching DNS provider. Mixed sets count as other.
func ProviderOf(ns []string) string {
	if len(ns) == 0 {
		return ProviderUnknown
	}
	kind := ""
	for _, n := range ns {
		h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(n)), ".")
		var k string
		switch {
		case strings.HasSuffix(h, ".ns.cloudflare.com"):
			k = ProviderCloudflare
		case strings.Contains(h, "awsdns-"):
			k = ProviderRoute53
		default:
			k = ProviderOther
		}
		if kind != "" && kind != k {
			return ProviderOther
		}
		kind = k
	}
	return kind
}

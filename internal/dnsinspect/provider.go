package dnsinspect

import "strings"

// Name server provider classes reported by ProviderOf.
const (
	ProviderCloudflare   = "cloudflare"
	ProviderRoute53      = "route53"
	ProviderDigitalOcean = "digitalocean"
	ProviderHetzner      = "hetzner"
	ProviderNamecheap    = "namecheap"
	ProviderGoDaddy      = "godaddy"
	ProviderPorkbun      = "porkbun"
	ProviderGoogle       = "google"
	ProviderAzure        = "azure"
	ProviderNS1          = "ns1"
	ProviderDeSEC        = "desec"
	ProviderOther        = "other"
	ProviderUnknown      = "unknown"
)

// suffixProviders maps a name server domain to its provider. Matching is on
// a dot boundary so look-alike hosts never classify.
var suffixProviders = []struct{ suffix, provider string }{
	{"digitalocean.com", ProviderDigitalOcean},
	{"ns.hetzner.com", ProviderHetzner},
	{"first-ns.de", ProviderHetzner},
	{"registrar-servers.com", ProviderNamecheap},
	{"namecheaphosting.com", ProviderNamecheap},
	{"domaincontrol.com", ProviderGoDaddy},
	{"porkbun.com", ProviderPorkbun},
	{"googledomains.com", ProviderGoogle},
	{"azure-dns.com", ProviderAzure},
	{"azure-dns.net", ProviderAzure},
	{"azure-dns.org", ProviderAzure},
	{"azure-dns.info", ProviderAzure},
	{"nsone.net", ProviderNS1},
	{"desec.io", ProviderDeSEC},
	{"desec.org", ProviderDeSEC},
}

func suffixProvider(h string) string {
	for _, sp := range suffixProviders {
		if strings.HasSuffix(h, "."+sp.suffix) {
			return sp.provider
		}
	}
	return ""
}

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
			if k = suffixProvider(h); k == "" {
				k = ProviderOther
			}
		}
		if kind != "" && kind != k {
			return ProviderOther
		}
		kind = k
	}
	return kind
}

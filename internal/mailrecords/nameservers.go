package mailrecords

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// DNS hosts the nameserver classifier knows. Anything else is HostManual.
const (
	HostCloudflare   = "cloudflare"
	HostRoute53      = "route53"
	HostDigitalOcean = "digitalocean"
	HostGoogleCloud  = "googlecloud"
	HostAzure        = "azure"
	HostHetzner      = "hetzner"
	HostNamecheap    = "namecheap"
	HostGoDaddy      = "godaddy"
	HostManual       = "manual"
)

// ManagedComment tags every record mail DNS creates where the provider keeps comments.
const ManagedComment = "managed by levelrail mail"

// NSResolver is the public resolver surface detection needs; net.Resolver satisfies it.
type NSResolver interface {
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// DomainConnect is discovery only: the API host the DNS provider publishes. Nothing calls it.
type DomainConnect struct {
	Supported bool   `json:"supported"`
	Host      string `json:"host,omitempty"`
	Path      string `json:"path,omitempty"`
}

// Detection is where a domain's DNS lives, from public NS and TXT lookups.
type Detection struct {
	Domain        string        `json:"domain"`
	Apex          string        `json:"apex"`
	Zone          string        `json:"zone"`
	Nameservers   []string      `json:"nameservers"`
	Provider      string        `json:"provider"`
	DomainConnect DomainConnect `json:"domain_connect"`
}

var nsSuffixes = []struct{ host, suffix string }{
	{HostCloudflare, ".ns.cloudflare.com"},
	{HostDigitalOcean, ".digitalocean.com"},
	{HostGoogleCloud, ".googledomains.com"},
	{HostAzure, ".azure-dns.com"}, {HostAzure, ".azure-dns.net"}, {HostAzure, ".azure-dns.org"}, {HostAzure, ".azure-dns.info"},
	{HostHetzner, ".hetzner.com"}, {HostHetzner, ".hetzner.de"}, {HostHetzner, ".your-server.de"}, {HostHetzner, ".second-ns.com"},
	{HostNamecheap, ".registrar-servers.com"},
	{HostGoDaddy, ".domaincontrol.com"},
}

// classifyOne names the host of one nameserver, or "" when it is unknown.
func classifyOne(ns string) string {
	ns = trimDot(ns)
	if strings.HasPrefix(ns, "ns-cloud-") && strings.HasSuffix(ns, ".googledomains.com") {
		return HostGoogleCloud
	}
	if strings.Contains(ns, ".awsdns-") {
		return HostRoute53
	}
	for _, s := range nsSuffixes {
		if strings.HasSuffix(ns, s.suffix) {
			return s.host
		}
	}
	return ""
}

// ClassifyNameservers names the DNS host when every nameserver agrees; mixed or unknown sets are manual.
func ClassifyNameservers(ns []string) string {
	host := ""
	for _, n := range ns {
		h := classifyOne(n)
		if h == "" || (host != "" && h != host) {
			return HostManual
		}
		host = h
	}
	if host == "" {
		return HostManual
	}
	return host
}

// RegistrableApex returns the public suffix plus one label (example.co.uk for a.example.co.uk).
func RegistrableApex(domain string) (string, error) {
	domain = trimDot(domain)
	if err := ValidHost(domain); err != nil {
		return "", err
	}
	apex, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		return "", fmt.Errorf("%q has no registrable domain: %w", domain, err)
	}
	return apex, nil
}

// FindZoneCut walks from domain up to its registrable apex and returns the
// first name that has NS records, so a delegated subdomain reports its own host.
func FindZoneCut(ctx context.Context, r NSResolver, domain, apex string) (string, []string, error) {
	name := trimDot(domain)
	for {
		recs, err := r.LookupNS(ctx, name)
		if err == nil && len(recs) > 0 {
			out := make([]string, 0, len(recs))
			for _, n := range recs {
				out = append(out, trimDot(n.Host))
			}
			slices.Sort(out)
			return name, slices.Compact(out), nil
		}
		if name == apex || !strings.Contains(name, ".") {
			return "", nil, fmt.Errorf("no nameservers found for %s or its parent domains", domain)
		}
		_, name, _ = strings.Cut(name, ".")
	}
}

var dcPath = regexp.MustCompile(`^[A-Za-z0-9/_.~-]*$`)

// ParseDomainConnect validates a _domainconnect TXT value: a host name with an optional path.
func ParseDomainConnect(values []string) (DomainConnect, bool) {
	for _, v := range values {
		v = strings.TrimSpace(v)
		host, path, _ := strings.Cut(v, "/")
		host = trimDot(host)
		if ValidHost(host) != nil || strings.Count(host, ".") < 1 || net.ParseIP(host) != nil || !dcPath.MatchString(path) || strings.Contains(path, "..") {
			continue
		}
		dc := DomainConnect{Supported: true, Host: host}
		if path != "" {
			dc.Path = "/" + path
		}
		return dc, true
	}
	return DomainConnect{}, false
}

// discoverDomainConnect reads _domainconnect at the registrable apex first, then at the zone cut.
func discoverDomainConnect(ctx context.Context, r NSResolver, apex, zone string) DomainConnect {
	names := []string{apex}
	if zone != "" && zone != apex {
		names = append(names, zone)
	}
	for _, n := range names {
		vals, err := r.LookupTXT(ctx, "_domainconnect."+n)
		if err != nil {
			continue
		}
		if dc, ok := ParseDomainConnect(vals); ok {
			return dc
		}
	}
	return DomainConnect{}
}

// Detect finds the zone and classifies its DNS host. It only resolves public DNS.
func Detect(ctx context.Context, r NSResolver, domain string) (Detection, error) {
	domain = trimDot(domain)
	apex, err := RegistrableApex(domain)
	if err != nil {
		return Detection{}, err
	}
	zone, ns, err := FindZoneCut(ctx, r, domain, apex)
	if err != nil {
		return Detection{Domain: domain, Apex: apex, Nameservers: []string{}, Provider: HostManual}, err
	}
	return Detection{
		Domain: domain, Apex: apex, Zone: zone, Nameservers: ns,
		Provider:      ClassifyNameservers(ns),
		DomainConnect: discoverDomainConnect(ctx, r, apex, zone),
	}, nil
}

var ptrSuffixes = []struct{ host, suffix string }{
	{"aws", ".amazonaws.com"}, {"aws", ".compute.amazonaws.com"},
	{HostHetzner, ".your-server.de"}, {HostHetzner, ".hetzner.com"}, {HostHetzner, ".clients.your-server.de"},
	{HostGoogleCloud, ".googleusercontent.com"},
	{HostAzure, ".cloudapp.azure.com"}, {HostAzure, ".cloudapp.net"},
	{"linode", ".linodeusercontent.com"}, {"linode", ".members.linode.com"},
	{"vultr", ".vultrusercontent.com"}, {"vultr", ".vultr.com"},
	{"ovh", ".ovh.net"}, {"ovh", ".ip-ovh.net"},
}

// HostFromPTR guesses the hosting provider from the PTR names of the server IP; "" when unknown.
func HostFromPTR(names []string) string {
	for _, n := range names {
		n = trimDot(n)
		for _, s := range ptrSuffixes {
			if strings.HasSuffix(n, s.suffix) {
				return s.host
			}
		}
	}
	return ""
}

var ptrHostNames = map[string]string{
	"aws": "AWS (EC2 Elastic IP reverse DNS request)", HostHetzner: "Hetzner (Robot or Cloud console, server networking)",
	HostGoogleCloud: "Google Cloud (the VM's external IP, PTR record field)", HostAzure: "Azure (public IP resource, reverse FQDN)",
	"linode": "Akamai Linode (network tab of the instance)", "vultr": "Vultr (server settings, IPv4 reverse DNS)", "ovh": "OVHcloud (IP management, reverse DNS)",
}

// PTRHint says where reverse DNS is set: always at the IP owner, never in the zone.
func PTRHint(host string) string {
	if name, ok := ptrHostNames[host]; ok {
		return "Reverse DNS (PTR) is set at the hosting provider that owns the server IP, here " + name + ", not in your DNS zone."
	}
	return "Reverse DNS (PTR) is set at the hosting provider that owns the server IP, not in your DNS zone or at the registrar."
}

// ptrHintFor resolves the mail host's first IP and turns its PTR into a hint.
func ptrHintFor(ctx context.Context, r Resolver, host string) string {
	if host == "" {
		return PTRHint("")
	}
	ips, err := r.LookupHost(ctx, host)
	if err != nil || len(ips) == 0 {
		return PTRHint("")
	}
	names, _ := r.LookupAddr(ctx, ips[0])
	return PTRHint(HostFromPTR(names))
}

package ingress

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// IPFormat is how a wildcard-DNS provider expects the address embedded in a hostname label.
type IPFormat string

// Supported IP encodings. Dashed works for every provider listed.
const (
	IPFormatDashed IPFormat = "dashed"
	IPFormatDotted IPFormat = "dotted"
)

// Provider ids. Built-in ids are the provider's domain; ProviderCustom is the operator's own wildcard.
const (
	ProviderSSLIP    = "sslip.io"
	ProviderNIP      = "nip.io"
	ProviderTraefik  = "traefik.me"
	ProviderCustom   = "custom"
	DefaultProvider  = ProviderSSLIP
	sampleHostLabel  = "resolution-check"
	maxBaseDomainLen = 253
)

// DNSProvider is one free wildcard-DNS service that maps a hostname with an embedded IP to that IP.
type DNSProvider struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Suffix   string   `json:"suffix"`
	IPFormat IPFormat `json:"ip_format"`
	// IPv4Only providers have no known IPv6 mapping, so an IPv6 public address yields no hostname.
	IPv4Only bool   `json:"ipv4_only"`
	Note     string `json:"note"`
}

// dnsProviders is the one registry: adding a provider is adding an entry here.
var dnsProviders = []DNSProvider{
	{ID: ProviderSSLIP, Name: "sslip.io", Suffix: "sslip.io", IPFormat: IPFormatDashed, Note: "Open source, supports IPv4 and IPv6. The default."},
	{ID: ProviderNIP, Name: "nip.io", Suffix: "nip.io", IPFormat: IPFormatDashed, Note: "Widely used, IPv4 and IPv6. Rate limits can apply on shared lookups."},
	{ID: ProviderTraefik, Name: "traefik.me", Suffix: "traefik.me", IPFormat: IPFormatDashed, IPv4Only: true, Note: "IPv4 only. Run by the Traefik community."},
}

// DNSProviders returns the built-in providers in display order.
func DNSProviders() []DNSProvider { return append([]DNSProvider(nil), dnsProviders...) }

// LookupDNSProvider finds a built-in provider by id, case-insensitively.
func LookupDNSProvider(id string) (DNSProvider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, p := range dnsProviders {
		if p.ID == id {
			return p, true
		}
	}
	return DNSProvider{}, false
}

// ProviderChoice selects a provider; CustomBase is used only with ProviderCustom. The zero value is the default provider.
type ProviderChoice struct {
	Provider   string `json:"provider"`
	CustomBase string `json:"custom_base_domain,omitempty"`
}

// Normalized fills in the default provider and lowercases the fields.
func (c ProviderChoice) Normalized() ProviderChoice {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	c.CustomBase = strings.ToLower(strings.TrimSpace(c.CustomBase))
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	if c.Provider != ProviderCustom {
		c.CustomBase = ""
	}
	return c
}

var baseDomainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateCustomBase accepts a DNS name under a registrable domain: not an IP, not a wildcard, not a public suffix.
func ValidateCustomBase(base string) error {
	base = strings.ToLower(strings.TrimSpace(base))
	switch {
	case base == "":
		return errors.New("a base domain is required, for example apps.example.com")
	case net.ParseIP(base) != nil:
		return errors.New("the base domain must be a DNS name, not an IP address")
	case len(base) > maxBaseDomainLen:
		return errors.New("the base domain is too long")
	}
	labels := strings.Split(base, ".")
	if len(labels) < 2 {
		return fmt.Errorf("%q needs at least two labels, for example apps.example.com", base)
	}
	for _, l := range labels {
		if !baseDomainLabel.MatchString(l) {
			return fmt.Errorf("%q is not a valid DNS name", base)
		}
	}
	if _, err := publicsuffix.EffectiveTLDPlusOne(base); err != nil {
		return fmt.Errorf("%q is a public suffix, use a domain you own such as apps.example.com", base)
	}
	return nil
}

// Validate reports whether c names a known provider and, for custom, a valid base domain.
func (c ProviderChoice) Validate() error {
	c = c.Normalized()
	if c.Provider == ProviderCustom {
		return ValidateCustomBase(c.CustomBase)
	}
	if _, ok := LookupDNSProvider(c.Provider); !ok {
		return fmt.Errorf("unknown DNS provider %q", c.Provider)
	}
	return nil
}

// Label is the provider domain or custom base, for messages.
func (c ProviderChoice) Label() string {
	c = c.Normalized()
	if c.Provider == ProviderCustom {
		return c.CustomBase
	}
	return c.Provider
}

// FallbackDomainFor is FallbackDomain for an explicit provider choice. An unusable choice yields no hostname.
func FallbackDomainFor(publicHost, serviceName string, c ProviderChoice) (string, bool) {
	label := sanitizeDNSLabel(serviceName)
	if label == "" {
		return "", false
	}
	if strings.EqualFold(strings.TrimSpace(publicHost), LocalHost) {
		return label + "." + LocalHost, true
	}
	c = c.Normalized()
	if c.Provider == ProviderCustom {
		if ValidateCustomBase(c.CustomBase) != nil {
			return "", false
		}
		return label + "." + c.CustomBase, true
	}
	p, ok := LookupDNSProvider(c.Provider)
	if !ok {
		return "", false
	}
	return providerHost(p, publicHost, label)
}

func providerHost(p DNSProvider, publicHost, label string) (string, bool) {
	ip := net.ParseIP(strings.TrimSpace(publicHost))
	if ip == nil || !isPubliclyRoutable(ip) {
		return "", false
	}
	if p.IPv4Only && ip.To4() == nil {
		return "", false
	}
	return label + "." + encodeIP(ip, p.IPFormat) + "." + p.Suffix, true
}

// SampleHost is the hostname a resolution test looks up for c.
func SampleHost(publicHost string, c ProviderChoice) (string, bool) {
	return FallbackDomainFor(publicHost, sampleHostLabel, c)
}

func encodeIP(ip net.IP, f IPFormat) string {
	if v4 := ip.To4(); v4 != nil {
		if f == IPFormatDotted {
			return v4.String()
		}
		return strings.ReplaceAll(v4.String(), ".", "-")
	}
	return strings.ReplaceAll(ip.String(), ":", "-")
}

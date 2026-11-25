package spec

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

const (
	maxDomainBytes = 253
	maxLabelBytes  = 63
)

// ValidateAppDomain returns the canonical form of an app domain: lowercase,
// no trailing dot, Unicode converted to its A-label. Only new input is
// rejected when invalid; a stored legacy U-label stays routable because the
// ingress reconciler normalizes through this function at compare time.
func ValidateAppDomain(domain string) (string, error) {
	d := strings.ToLower(domain)
	d = strings.TrimSuffix(d, ".")
	if d == "" {
		return "", errors.New("domain is empty")
	}
	for _, r := range d {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("{}/:\\@%", r) {
			return "", fmt.Errorf("domain %q contains a forbidden character", domain)
		}
	}
	if _, err := netip.ParseAddr(strings.Trim(d, "[]")); err == nil {
		return "", fmt.Errorf("domain %q must be a hostname, not an IP address", domain)
	}
	prefix := ""
	base := d
	if strings.HasPrefix(d, "*.") {
		prefix, base = "*.", d[2:]
		if !strings.Contains(base, ".") {
			return "", fmt.Errorf("domain %q: wildcard base domain needs at least one more label", domain)
		}
	}
	if strings.Contains(base, "*") {
		return "", fmt.Errorf("domain %q: wildcard must be a single leading \"*.\" label", domain)
	}
	ascii, err := idna.Lookup.ToASCII(base)
	if err != nil {
		return "", fmt.Errorf("domain %q is not a valid hostname", domain)
	}
	out := prefix + ascii
	if len(out) > maxDomainBytes {
		return "", fmt.Errorf("domain exceeds %d bytes", maxDomainBytes)
	}
	for _, label := range strings.Split(ascii, ".") {
		if !validHostLabel(label) {
			return "", fmt.Errorf("domain %q has an invalid label", domain)
		}
	}
	return out, nil
}

// validHostLabel enforces RFC 1123: letters, digits and inner hyphens, 1 to 63 bytes.
func validHostLabel(l string) bool {
	if l == "" || len(l) > maxLabelBytes || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		isAlnum := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !isAlnum && c != '-' {
			return false
		}
	}
	return true
}

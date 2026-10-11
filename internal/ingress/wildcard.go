package ingress

import (
	"fmt"
	"strings"
)

// IsWildcardDomain reports whether domain is a wildcard hostname (e.g.
// "*.example.com"). A leading "*." is the whole convention: no separate
// schema field marks a domain wildcard-eligible.
func IsWildcardDomain(domain string) bool {
	return strings.HasPrefix(domain, "*.")
}

// ValidateWildcardDomain rejects a malformed wildcard: a "*" anywhere
// but the single leading "*." label, or a base domain with no further
// label for a DNS-01 TXT record to attach under. Non-wildcard domains
// and syntactically valid wildcards both return nil.
func ValidateWildcardDomain(domain string) error {
	if !strings.Contains(domain, "*") {
		return nil
	}
	if !IsWildcardDomain(domain) {
		return fmt.Errorf("ingress: %q: wildcard must be a single leading \"*.\" label", domain)
	}
	base := strings.TrimPrefix(domain, "*.")
	if base == "" || strings.Contains(base, "*") {
		return fmt.Errorf("ingress: %q: wildcard must be a single leading \"*.\" label", domain)
	}
	if !strings.Contains(base, ".") {
		return fmt.Errorf("ingress: %q: wildcard base domain needs at least one more label", domain)
	}
	return nil
}

// splitWildcardHosts partitions hosts into wildcard and non-wildcard
// subjects, preserving each group's relative order.
func splitWildcardHosts(hosts []string) (wildcard, regular []string) {
	for _, h := range hosts {
		if IsWildcardDomain(h) {
			wildcard = append(wildcard, h)
		} else {
			regular = append(regular, h)
		}
	}
	return wildcard, regular
}

// exactHostsFirst moves routes matching any wildcard host behind the exact
// ones, keeping relative order. Caddy runs routes in order, so without this
// a "*.example.com" route could answer for an app that owns api.example.com.
func exactHostsFirst(routes []Route) []Route {
	exact := make([]Route, 0, len(routes))
	var wild []Route
	for _, r := range routes {
		if routeHasWildcard(r) {
			wild = append(wild, r)
		} else {
			exact = append(exact, r)
		}
	}
	return append(exact, wild...)
}

func routeHasWildcard(r Route) bool {
	for _, m := range r.Match {
		for _, h := range m.Host {
			if IsWildcardDomain(h) {
				return true
			}
		}
	}
	return false
}

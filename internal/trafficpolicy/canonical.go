package trafficpolicy

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Canonical host presets.
const (
	PresetWWWToApex = "www-to-apex"
	PresetApexToWWW = "apex-to-www"
	PresetServeBoth = "both"
)

// Counterpart returns the www or apex twin of domain.
func Counterpart(domain string) (string, error) {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	if strings.HasPrefix(d, "*.") {
		return "", errors.New("a wildcard host has no www or apex counterpart")
	}
	if rest, ok := strings.CutPrefix(d, "www."); ok {
		if !strings.Contains(rest, ".") {
			return "", fmt.Errorf("%s has no apex to redirect to", d)
		}
		return rest, nil
	}
	return "www." + d, nil
}

// CanonicalPlan returns the redirect a preset installs: requests to from go
// to to. Serve both returns two empty strings.
func CanonicalPlan(domain, preset string) (from, to string, err error) {
	twin, err := Counterpart(domain)
	if err != nil {
		return "", "", err
	}
	www, apex := domain, twin
	if !strings.HasPrefix(strings.ToLower(domain), "www.") {
		www, apex = twin, domain
	}
	switch preset {
	case PresetWWWToApex:
		return strings.ToLower(www), strings.ToLower(apex), nil
	case PresetApexToWWW:
		return strings.ToLower(apex), strings.ToLower(www), nil
	case PresetServeBoth:
		return "", "", nil
	}
	return "", "", fmt.Errorf("preset must be %s, %s or %s", PresetWWWToApex, PresetApexToWWW, PresetServeBoth)
}

// TargetHost returns the lower case host of a redirect target URL.
func TargetHost(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// CheckRedirectLoop reports whether adding from -> toHost to rows would
// make a chain that returns to from, or exceeds MaxHops.
func CheckRedirectLoop(rows map[string]HostRedirect, from, toHost string) error {
	if from == toHost {
		return fmt.Errorf("%s cannot redirect to itself", from)
	}
	host := toHost
	for i := 0; i < MaxHops; i++ {
		r, ok := rows[host]
		if !ok {
			return nil
		}
		next := TargetHost(r.TargetURL)
		if next == from {
			return fmt.Errorf("%s already redirects back to %s, which would loop", toHost, from)
		}
		host = next
	}
	return fmt.Errorf("redirect chain from %s is longer than %d hops", toHost, MaxHops)
}

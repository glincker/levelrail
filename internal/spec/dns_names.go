package spec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DNSNameError carries a frozen code; internal/dnsnames aliases it. The core
// rules live here so low-level packages need not import internal/dnsnames.
type DNSNameError struct {
	Code string
	Msg  string
}

func (e *DNSNameError) Error() string { return e.Msg }

// DNSCodeInvalid is the frozen code for a malformed extra DNS name.
const DNSCodeInvalid = "dns_name_invalid"

func dnsFail(code, format string, args ...any) error {
	return &DNSNameError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

var dnsLabelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// NormalizeDNSNames lowercases, drops a trailing dot and the mesh zone suffix,
// then dedupes and sorts so stored, declared and live lists compare equal.
func NormalizeDNSNames(names []string, zone string) []string {
	if len(names) == 0 {
		return nil
	}
	suffix := "." + strings.ToLower(strings.Trim(zone, "."))
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(n)), ".")
		if zone != "" {
			n = strings.TrimSuffix(n, suffix)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// ValidateDNSNameSyntax checks strict lowercase LDH labels and refuses wildcards.
func ValidateDNSNameSyntax(names []string) error {
	for _, n := range names {
		if n == "" {
			return dnsFail(DNSCodeInvalid, "dns name must not be empty")
		}
		if strings.Contains(n, "*") {
			return dnsFail(DNSCodeInvalid, "dns name %q: wildcards are not supported", n)
		}
		for _, label := range strings.Split(n, ".") {
			if !dnsLabelRe.MatchString(label) {
				return dnsFail(DNSCodeInvalid, "dns name %q: each label must be lowercase letters, digits or hyphens, not starting or ending with a hyphen", n)
			}
		}
	}
	return nil
}

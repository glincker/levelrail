// Package dnsnames validates the extra internal DNS names an app can claim.
// Pure functions so the spec, the API and the IaC planner share one rule set.
package dnsnames

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// Frozen error codes, mirrored by internal/iac/codes.go.
const (
	CodeInvalid  = spec.DNSCodeInvalid
	CodeReserved = "dns_name_reserved"
	CodeConflict = "dns_name_conflict"
)

// Error carries a frozen code so callers can surface it unchanged.
type Error = spec.DNSNameError

func fail(code, format string, args ...any) error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Limits are caps read from runtime settings; a zero cap means unchecked.
type Limits struct {
	MaxNames    int
	MaxLabelLen int
	MaxNameLen  int
	Reserved    map[string]bool
}

// ParseReserved turns a comma separated setting value into a lookup set.
func ParseReserved(csv string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(csv, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out[p] = true
		}
	}
	return out
}

// Normalize delegates to the shared rule in internal/spec.
func Normalize(names []string, zone string) []string { return spec.NormalizeDNSNames(names, zone) }

// ValidateSyntax delegates to the shared rule in internal/spec.
func ValidateSyntax(names []string) error { return spec.ValidateDNSNameSyntax(names) }

// Validate applies syntax, the setting caps and the reserved list.
func Validate(names []string, l Limits) error {
	if err := ValidateSyntax(names); err != nil {
		return err
	}
	if l.MaxNames > 0 && len(names) > l.MaxNames {
		return fail(CodeInvalid, "at most %d dns names per app", l.MaxNames)
	}
	for _, n := range names {
		if l.MaxNameLen > 0 && len(n) > l.MaxNameLen {
			return fail(CodeInvalid, "dns name %q is longer than %d characters", n, l.MaxNameLen)
		}
		if l.MaxLabelLen > 0 {
			for _, label := range strings.Split(n, ".") {
				if len(label) > l.MaxLabelLen {
					return fail(CodeInvalid, "dns label %q is longer than %d characters", label, l.MaxLabelLen)
				}
			}
		}
		if l.Reserved[n] {
			return fail(CodeReserved, "dns name %q is reserved", n)
		}
	}
	return nil
}

// Conflict reports a name already taken by another app. taken maps a name,
// either an app name or another app's extra name, to its owner.
func Conflict(names []string, self string, taken map[string]string) error {
	for _, n := range names {
		if owner, ok := taken[n]; ok && owner != self {
			return fail(CodeConflict, "dns name %q is already used by %q", n, owner)
		}
	}
	return nil
}

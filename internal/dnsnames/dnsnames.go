// Package dnsnames validates the extra internal DNS names an app can claim.
// Pure functions so the spec, the API and the IaC planner share one rule set.
package dnsnames

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Frozen error codes, mirrored by internal/iac/codes.go.
const (
	CodeInvalid  = "dns_name_invalid"
	CodeReserved = "dns_name_reserved"
	CodeConflict = "dns_name_conflict"
)

// Error carries a frozen code so callers can surface it unchanged.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func fail(code, format string, args ...any) error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

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

// Normalize lowercases, drops a trailing dot and the mesh zone suffix, then
// dedupes and sorts so stored, declared and live lists compare equal.
func Normalize(names []string, zone string) []string {
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

// ValidateSyntax checks strict lowercase LDH labels and refuses wildcards.
func ValidateSyntax(names []string) error {
	for _, n := range names {
		if n == "" {
			return fail(CodeInvalid, "dns name must not be empty")
		}
		if strings.Contains(n, "*") {
			return fail(CodeInvalid, "dns name %q: wildcards are not supported", n)
		}
		for _, label := range strings.Split(n, ".") {
			if !labelRe.MatchString(label) {
				return fail(CodeInvalid, "dns name %q: each label must be lowercase letters, digits or hyphens, not starting or ending with a hyphen", n)
			}
		}
	}
	return nil
}

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

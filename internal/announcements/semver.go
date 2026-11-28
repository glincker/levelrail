package announcements

import (
	"fmt"
	"strconv"
	"strings"
)

type semver [3]int

// parseVersion reads vX.Y.Z, ignoring any pre-release or build suffix.
// Builds such as "dev" are not versions and never match a range.
func parseVersion(s string) (semver, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var v semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		v[i] = n
	}
	return v, true
}

func (a semver) cmp(b semver) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

type comparator struct {
	op  string
	ver semver
}

// VersionRange is an AND of comparators such as ">=1.5.0,<1.5.14".
type VersionRange []comparator

var rangeOps = []string{">=", "<=", "<", ">", "="}

// ParseRange parses a comma or space separated list of comparators. An empty
// range or a bare wildcard is an error, so a targeted item cannot hit everyone.
func ParseRange(s string) (VersionRange, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return nil, fmt.Errorf("announcements: empty version range")
	}
	var out VersionRange
	for _, f := range fields {
		op := "="
		for _, o := range rangeOps {
			if strings.HasPrefix(f, o) {
				op, f = o, strings.TrimPrefix(f, o)
				break
			}
		}
		v, ok := parseVersion(f)
		if !ok {
			return nil, fmt.Errorf("announcements: bad version %q in range %q", f, s)
		}
		out = append(out, comparator{op: op, ver: v})
	}
	return out, nil
}

// Match reports whether version satisfies every comparator.
func (r VersionRange) Match(version string) bool {
	v, ok := parseVersion(version)
	if !ok {
		return false
	}
	for _, c := range r {
		d := v.cmp(c.ver)
		var pass bool
		switch c.op {
		case ">=":
			pass = d >= 0
		case "<=":
			pass = d <= 0
		case "<":
			pass = d < 0
		case ">":
			pass = d > 0
		default:
			pass = d == 0
		}
		if !pass {
			return false
		}
	}
	return true
}

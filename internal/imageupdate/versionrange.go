package imageupdate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is a numeric major.minor.patch parsed from an image tag.
type Version [3]int

func (v Version) cmp(o Version) int {
	for i := range v {
		switch {
		case v[i] < o[i]:
			return -1
		case v[i] > o[i]:
			return 1
		}
	}
	return 0
}

type comparator struct {
	op string
	v  Version
}

// Range is a set of comparators that must all hold, parsed from a spec such
// as "1.x", "1.2.x", "^1.2.3", "~1.2.3" or ">=1.2.0 <2.0.0".
type Range struct{ cs []comparator }

var (
	cmpSpec  = regexp.MustCompile(`^(>=|<=|>|<|=)?v?(\d+)(?:\.(\d+))?(?:\.(\d+))?$`)
	wildSpec = regexp.MustCompile(`^v?(\d+)(?:\.(\d+|[xX*]))?(?:\.(\d+|[xX*]))?$`)
)

// ParseRange validates spec. An empty spec or "*" matches every version.
func ParseRange(spec string) (Range, error) {
	var r Range
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "*" {
		return r, nil
	}
	for _, tok := range strings.Fields(spec) {
		cs, err := parseToken(tok)
		if err != nil {
			// Quote the whole value so the caller sees what they sent.
			return Range{}, fmt.Errorf("invalid version range %q", spec)
		}
		r.cs = append(r.cs, cs...)
	}
	return r, nil
}

func parseToken(tok string) ([]comparator, error) {
	bad := fmt.Errorf("invalid version range %q", tok)
	switch {
	case strings.HasPrefix(tok, "^"), strings.HasPrefix(tok, "~"):
		v, ok := numericParts(tok[1:])
		if !ok {
			return nil, bad
		}
		upper := Version{v[0] + 1}
		switch {
		case tok[0] == '~':
			upper = Version{v[0], v[1] + 1}
		case v[0] == 0:
			upper = Version{0, v[1] + 1}
		}
		return []comparator{{">=", v}, {"<", upper}}, nil
	}
	if m := wildSpec.FindStringSubmatch(tok); m != nil {
		// "1", "1.x" and "1.2.x" pin the leading parts and float the rest.
		var v Version
		fixed := 0
		for i := 1; i <= 3; i++ {
			if m[i] == "" || strings.ContainsAny(m[i], "xX*") {
				break
			}
			v[i-1], _ = strconv.Atoi(m[i])
			fixed++
		}
		if fixed == 3 {
			return []comparator{{"=", v}}, nil
		}
		upper := v
		if fixed == 1 {
			upper[0]++
		} else {
			upper[1]++
		}
		return []comparator{{">=", v}, {"<", upper}}, nil
	}
	if m := cmpSpec.FindStringSubmatch(tok); m != nil && m[1] != "" {
		v, _ := numericParts(strings.TrimLeft(tok, "<>="))
		return []comparator{{m[1], v}}, nil
	}
	return nil, bad
}

func numericParts(s string) (Version, bool) {
	m := cmpSpec.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	var v Version
	for i := 0; i < 3; i++ {
		if m[i+2] != "" {
			v[i], _ = strconv.Atoi(m[i+2])
		}
	}
	return v, true
}

// Contains reports whether v satisfies every comparator.
func (r Range) Contains(v Version) bool {
	for _, c := range r.cs {
		d := v.cmp(c.v)
		ok := false
		switch c.op {
		case ">=":
			ok = d >= 0
		case ">":
			ok = d > 0
		case "<=":
			ok = d <= 0
		case "<":
			ok = d < 0
		default:
			ok = d == 0
		}
		if !ok {
			return false
		}
	}
	return true
}

// tagPattern captures a release tag: optional v, up to three numbers, and an
// optional variant suffix such as "-alpine".
var tagPattern = regexp.MustCompile(`^(v?)(\d+)(?:\.(\d+))?(?:\.(\d+))?(-[A-Za-z][A-Za-z0-9._-]*)?$`)

// prereleaseWord marks suffixes that are pre-releases rather than variants.
var prereleaseWord = regexp.MustCompile(`(?i)^-(alpha|beta|rc|pre|preview|dev|nightly|snapshot|canary|edge|test)`)

// ParseTag splits a tag into its version and variant suffix. ok is false for
// tags that are not release versions (latest, stable) and for pre-releases.
func ParseTag(tag string) (v Version, suffix string, ok bool) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil || prereleaseWord.MatchString(m[5]) {
		return Version{}, "", false
	}
	for i := 0; i < 3; i++ {
		if m[i+2] != "" {
			v[i], _ = strconv.Atoi(m[i+2])
		}
	}
	return v, m[5], true
}

// NewestInRange returns the highest release tag inside r that carries the
// same variant suffix as currentTag (so "1.25-alpine" never jumps to a plain
// tag). ok is false when no tag qualifies.
func NewestInRange(tags []string, r Range, currentTag string) (best string, ok bool) {
	_, wantSuffix, _ := ParseTag(currentTag)
	var bestV Version
	for _, t := range tags {
		v, suffix, valid := ParseTag(t)
		if !valid || suffix != wantSuffix || !r.Contains(v) {
			continue
		}
		if !ok || v.cmp(bestV) > 0 {
			best, bestV, ok = t, v, true
		}
	}
	return best, ok
}

// IsNewer reports whether candidate is a higher release version than current.
// A current tag that is not a release version (latest) counts as lower.
func IsNewer(candidate, current string) bool {
	cv, _, cok := ParseTag(candidate)
	if !cok {
		return false
	}
	curV, _, ok := ParseTag(current)
	return !ok || cv.cmp(curV) > 0
}

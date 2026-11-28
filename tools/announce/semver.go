package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type semver struct {
	maj, min, pat int
	pre           string
}

var semverRe = regexp.MustCompile(`^v?(\d{1,6})\.(\d{1,6})\.(\d{1,6})(?:-([0-9A-Za-z.-]{1,32}))?$`)

func parseSemver(s string) (semver, error) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return semver{}, fmt.Errorf("not a semver: %q", s)
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	return semver{a, b, c, m[4]}, nil
}

// cmp returns -1, 0 or 1. A prerelease sorts below its release.
func (a semver) cmp(b semver) int {
	for _, p := range [][2]int{{a.maj, b.maj}, {a.min, b.min}, {a.pat, b.pat}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.pre == b.pre:
		return 0
	case a.pre == "":
		return 1
	case b.pre == "":
		return -1
	}
	return strings.Compare(a.pre, b.pre)
}

type constraint struct {
	op string
	v  semver
}

var rangeTokenRe = regexp.MustCompile(`^(<=|>=|<|>|=)?(.+)$`)

// parseRange accepts space or comma separated constraints such as
// ">=1.5.0 <1.5.14". Wildcards and empty ranges are rejected.
func parseRange(s string) ([]constraint, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
	if len(fields) == 0 || len(fields) > 4 {
		return nil, fmt.Errorf("applies_to needs 1 to 4 constraints")
	}
	out := make([]constraint, 0, len(fields))
	for _, f := range fields {
		m := rangeTokenRe.FindStringSubmatch(f)
		op := m[1]
		if op == "" {
			op = "="
		}
		v, err := parseSemver(m[2])
		if err != nil {
			return nil, err
		}
		out = append(out, constraint{op, v})
	}
	return out, nil
}

func (c constraint) holds(v semver) bool {
	r := v.cmp(c.v)
	switch c.op {
	case "<":
		return r < 0
	case "<=":
		return r <= 0
	case ">":
		return r > 0
	case ">=":
		return r >= 0
	}
	return r == 0
}

func rangeMatches(cs []constraint, v semver) bool {
	for _, c := range cs {
		if !c.holds(v) {
			return false
		}
	}
	return true
}

// satisfiable reports whether some version can match every constraint,
// probing each bound and the points just beside it.
func satisfiable(cs []constraint) bool {
	for _, c := range cs {
		cands := []semver{c.v, {c.v.maj, c.v.min, c.v.pat + 1, ""}, {c.v.maj, c.v.min, 0, "0"}}
		if c.v.pat > 0 {
			cands = append(cands, semver{c.v.maj, c.v.min, c.v.pat - 1, ""})
		}
		for _, v := range cands {
			if rangeMatches(cs, v) {
				return true
			}
		}
	}
	return false
}

// Package dbupgrade advises on and automates minor and patch upgrades of
// managed databases: a curated version catalog, per database policies with a
// maintenance window, and a resumable runner that backs up, upgrades,
// verifies and reverts.
package dbupgrade

import (
	"strconv"
	"strings"
)

// Upgrade kinds, ordered by risk.
const (
	KindPatch = "patch"
	KindMinor = "minor"
	KindMajor = "major"
)

func kindRank(kind string) int {
	switch kind {
	case KindPatch:
		return 1
	case KindMinor:
		return 2
	case KindMajor:
		return 3
	}
	return 0
}

// version is a parsed image tag: optional "v" prefix, dotted numbers, and
// an optional "-suffix" (a flavour such as "alpine").
type version struct {
	raw    string
	prefix string
	nums   []int
	suffix string
}

func parseVersion(raw string) (version, bool) {
	v := version{raw: raw}
	s := raw
	if strings.HasPrefix(s, "v") {
		v.prefix = "v"
		s = s[1:]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.suffix = s[i+1:]
		s = s[:i]
	}
	if s == "" {
		return version{}, false
	}
	for _, part := range strings.Split(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, false
		}
		v.nums = append(v.nums, n)
	}
	return v, true
}

// compareVersions orders by numeric components; a missing component counts as 0.
func compareVersions(a, b version) int {
	n := len(a.nums)
	if len(b.nums) > n {
		n = len(b.nums)
	}
	for i := 0; i < n; i++ {
		x, y := at(a.nums, i), at(b.nums, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func at(nums []int, i int) int {
	if i < len(nums) {
		return nums[i]
	}
	return 0
}

// classify names the level of the first component that differs, using the
// engine's per component levels; components past the table are patches.
func classify(levels []string, from, to version) string {
	n := len(from.nums)
	if len(to.nums) > n {
		n = len(to.nums)
	}
	for i := 0; i < n; i++ {
		if at(from.nums, i) != at(to.nums, i) {
			if i < len(levels) {
				return levels[i]
			}
			return KindPatch
		}
	}
	return ""
}

// lineKey is the version's support line: every leading component marked major.
func lineKey(levels []string, v version) string {
	depth := 0
	for i, l := range levels {
		if l == KindMajor {
			depth = i + 1
		}
	}
	if depth == 0 {
		depth = 1
	}
	parts := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		parts = append(parts, strconv.Itoa(at(v.nums, i)))
	}
	return strings.Join(parts, ".")
}

// seriesKey is every component above patch level ("7.2" for Redis 7.2.11,
// "16" for Postgres 16.11): the release series a security fix lands in.
func seriesKey(levels []string, v version) string {
	depth := 0
	for i, l := range levels {
		if l != KindPatch {
			depth = i + 1
		}
	}
	if depth == 0 {
		depth = 1
	}
	parts := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		parts = append(parts, strconv.Itoa(at(v.nums, i)))
	}
	return strings.Join(parts, ".")
}

// groupKey buckets targets so the advisor offers the newest patch, the
// newest release of each newer minor, and the newest of each newer major line.
func groupKey(levels []string, from, to version) string {
	switch kind := classify(levels, from, to); kind {
	case KindPatch:
		return KindPatch
	case KindMajor:
		return KindMajor + ":" + lineKey(levels, to)
	default:
		n := len(to.nums)
		parts := make([]string, 0, n)
		for i := 0; i < n; i++ {
			parts = append(parts, strconv.Itoa(at(to.nums, i)))
			if at(from.nums, i) != at(to.nums, i) {
				break
			}
		}
		return kind + ":" + strings.Join(parts, ".")
	}
}

// floating reports whether current names fewer components than the catalog
// pins, so the running image may be any release of that line.
func floating(current version, catalog []version) bool {
	most := 0
	for _, c := range catalog {
		if len(c.nums) > most {
			most = len(c.nums)
		}
	}
	return most > 0 && len(current.nums) < most
}

func (v version) tagWithSuffix(suffix string) string {
	if suffix == "" {
		return v.raw
	}
	return v.raw + "-" + suffix
}

package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/distribution/reference"
)

// ImageRef is an image reference split into what a tag listing needs.
type ImageRef struct {
	// Registry is the registry host; "docker.io" for Docker Hub.
	Registry string
	// Repository is the path inside the registry, "library/postgres" for "postgres".
	Repository string
	// Tag is empty when the reference carries only a digest.
	Tag    string
	Digest string
}

// ParseImageRef splits an image reference using the same rules the
// container runtime applies to short names.
func ParseImageRef(ref string) (ImageRef, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ImageRef{}, fmt.Errorf("catalog: parse image %q: %w", ref, err)
	}
	out := ImageRef{Registry: reference.Domain(named), Repository: reference.Path(named)}
	if t, ok := named.(reference.Tagged); ok {
		out.Tag = t.Tag()
	}
	if d, ok := named.(reference.Digested); ok {
		out.Digest = d.Digest().String()
	}
	return out, nil
}

// IsFloatingTag reports whether a tag names no fixed version, so only a
// digest comparison can tell whether it moved.
func IsFloatingTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "", "latest", "stable", "main", "master", "lts", "edge", "nightly", "release":
		return true
	}
	return false
}

var versionPattern = regexp.MustCompile(`^(.*?)(\d+(?:\.\d+)*)(.*)$`)

type tagVersion struct {
	prefix, suffix string
	nums           []int
}

func parseTagVersion(tag string) (tagVersion, bool) {
	m := versionPattern.FindStringSubmatch(tag)
	if m == nil {
		return tagVersion{}, false
	}
	var nums []int
	for _, part := range strings.Split(m[2], ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return tagVersion{}, false
		}
		nums = append(nums, n)
	}
	return tagVersion{prefix: m[1], suffix: m[3], nums: nums}, true
}

func (v tagVersion) newerThan(o tagVersion) bool {
	for i := 0; i < len(v.nums) || i < len(o.nums); i++ {
		var a, b int
		if i < len(v.nums) {
			a = v.nums[i]
		}
		if i < len(o.nums) {
			b = o.nums[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

// NewerTags returns the candidates that are a later version of current
// with the same prefix and suffix (so "16-alpine" is compared only with
// other "-alpine" tags), newest first, at most limit of them.
func NewerTags(current string, candidates []string, limit int) []string {
	cur, ok := parseTagVersion(current)
	if !ok {
		return nil
	}
	type scored struct {
		tag string
		v   tagVersion
	}
	var found []scored
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] || c == current {
			continue
		}
		seen[c] = true
		v, ok := parseTagVersion(c)
		if !ok || v.prefix != cur.prefix || v.suffix != cur.suffix || !v.newerThan(cur) {
			continue
		}
		found = append(found, scored{tag: c, v: v})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].v.newerThan(found[j].v) })
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.tag
	}
	return out
}

package advisoryfeed

import (
	"sort"
	"strconv"
	"strings"
)

// Image is one running image of an app. Digest may be empty.
type Image struct {
	App    string `json:"app"`
	Ref    string `json:"image"`
	Digest string `json:"digest,omitempty"`
}

// Match is one advisory that applies to one running image.
type Match struct {
	App        string `json:"app"`
	Image      string `json:"image"`
	Digest     string `json:"digest,omitempty"`
	AdvisoryID string `json:"advisory_id"`
	Severity   string `json:"severity"`
	Summary    string `json:"summary"`
	FixedIn    string `json:"fixed_in,omitempty"`
	Reference  string `json:"reference,omitempty"`
	Hint       string `json:"hint"`
}

// NormalizeRepository lowercases and strips the default registry and library
// prefixes so "node" and "docker.io/library/node" compare equal.
func NormalizeRepository(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"index.docker.io/", "registry-1.docker.io/", "docker.io/"} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimPrefix(s, "library/")
}

// splitRef returns repository, tag and digest of an image reference.
func splitRef(ref string) (repo, tag, digest string) {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "@"); i >= 0 {
		digest = ref[i+1:]
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		tag = ref[i+1:]
		ref = ref[:i]
	}
	return NormalizeRepository(ref), tag, strings.ToLower(digest)
}

// MatchImages compares every image against every advisory. The result is
// ordered worst severity first, then app, then advisory id.
func MatchImages(f Feed, images []Image) []Match {
	var out []Match
	for _, img := range images {
		repo, tag, refDigest := splitRef(img.Ref)
		digest := strings.ToLower(img.Digest)
		if digest == "" {
			digest = refDigest
		}
		for _, e := range f.Entries {
			if entryHits(e, repo, tag, digest) {
				out = append(out, newMatch(e, img, digest))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := SeverityRank(a.Severity), SeverityRank(b.Severity); ra != rb {
			return ra > rb
		}
		if a.App != b.App {
			return a.App < b.App
		}
		return a.AdvisoryID < b.AdvisoryID
	})
	return out
}

func newMatch(e Entry, img Image, digest string) Match {
	hint := "Rebuild or redeploy the app on a fixed image."
	if e.FixedIn != "" {
		hint = "Move to " + e.FixedIn + " or newer, then redeploy."
	}
	return Match{
		App: img.App, Image: img.Ref, Digest: digest, AdvisoryID: e.ID, Severity: e.Severity,
		Summary: e.Summary, FixedIn: e.FixedIn, Reference: e.Reference, Hint: hint,
	}
}

func entryHits(e Entry, repo, tag, digest string) bool {
	for _, a := range e.Affected {
		if NormalizeRepository(a.Repository) != repo {
			continue
		}
		for _, d := range a.Digests {
			if digest != "" && strings.EqualFold(d, digest) {
				return true
			}
		}
		for _, t := range a.Tags {
			if tag != "" && t == tag {
				return true
			}
		}
		if tag != "" && inRanges(a.Ranges, tag) {
			return true
		}
	}
	return false
}

func inRanges(ranges []Range, tag string) bool {
	v, ok := parseVersion(tag)
	if !ok {
		return false
	}
	for _, r := range ranges {
		before, _ := parseVersion(r.Before)
		if compareVersions(v, before) >= 0 {
			continue
		}
		if r.From != "" {
			from, _ := parseVersion(r.From)
			if compareVersions(v, from) < 0 {
				continue
			}
		}
		return true
	}
	return false
}

// parseVersion reads a leading dotted number such as "v20.11.1-alpine".
func parseVersion(s string) ([]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+_"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func compareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

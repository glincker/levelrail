package dbupgrade

import (
	"sort"
	"time"
)

// Support statuses of a database's current release line.
const (
	SupportSupported = "supported"
	SupportEOLSoon   = "eol_soon"
	SupportEOL       = "eol"
	SupportUnknown   = "unknown"
)

// Target is one version a database could move to.
type Target struct {
	Version  string `json:"version"`
	Kind     string `json:"kind"`
	Security bool   `json:"security"`
	// Advisories are the CVE ids this target fixes for the current version.
	Advisories []string `json:"advisories,omitempty"`
	EOL        string   `json:"eol,omitempty"`
	// Automatic is true when the engine allows this kind without an operator.
	Automatic bool `json:"automatic"`
}

// Advice is the advisor's view of one database.
type Advice struct {
	Engine         string   `json:"engine"`
	Current        string   `json:"current"`
	Comparable     bool     `json:"comparable"`
	Floating       bool     `json:"floating"`
	Line           string   `json:"line,omitempty"`
	EOL            string   `json:"eol,omitempty"`
	Support        string   `json:"support"`
	Advisories     []string `json:"advisories,omitempty"`
	Targets        []Target `json:"targets"`
	AutoMax        string   `json:"auto_max"`
	ImageRevert    bool     `json:"image_revert"`
	ManualReason   string   `json:"manual_reason,omitempty"`
	Notes          string   `json:"notes,omitempty"`
	Note           string   `json:"note,omitempty"`
	CatalogUpdated string   `json:"catalog_updated"`
}

// HasSecurityUpdate reports whether any target fixes an advisory.
func (a Advice) HasSecurityUpdate() bool {
	for _, t := range a.Targets {
		if t.Security {
			return true
		}
	}
	return false
}

// Advisor computes upgrade targets from a Catalog.
type Advisor struct {
	Catalog *Catalog
	// EOLWarn is how long before end of life a line counts as eol_soon.
	EOLWarn time.Duration
}

// Advise returns the targets for a database running engine at current.
func (ad Advisor) Advise(engine, current string, now time.Time) Advice {
	out := Advice{Engine: engine, Current: current, Support: SupportUnknown, Targets: []Target{}, CatalogUpdated: ad.Catalog.Updated()}
	ec, ok := ad.Catalog.Engine(engine)
	if !ok {
		out.Note = "this engine is not in the upgrade catalog"
		out.AutoMax = autoNone
		return out
	}
	out.AutoMax, out.ImageRevert, out.ManualReason, out.Notes = ec.AutoMax, ec.ImageRevert, ec.ManualReason, ec.Notes
	cur, ok := parseVersion(current)
	if !ok {
		out.Note = "the current tag is not a numeric version, so newer releases cannot be compared; pin a version such as the engine's latest patch"
		return out
	}
	if cur.suffix == "pgvector" {
		out.Note = "the pgvector image follows a major line only; it has no pinned patch tags to move to"
		return out
	}
	out.Comparable = true
	catalog := parseAll(ec.Versions)
	out.Floating = floating(cur, catalog)
	if out.Floating {
		out.Note = "the tag floats on its line, so the running release is unknown and treated as the oldest of that line; upgrading pins it"
	}
	out.Line = lineKey(ec.Levels, cur)
	out.EOL, out.Support = support(ec.Lines, out.Line, now, ad.EOLWarn)
	out.Advisories = affecting(ec, cur)

	best := map[string]version{}
	for _, cand := range catalog {
		if compareVersions(cand, cur) <= 0 || cand.prefix != cur.prefix {
			continue
		}
		key := groupKey(ec.Levels, cur, cand)
		if prev, seen := best[key]; !seen || compareVersions(cand, prev) > 0 {
			best[key] = cand
		}
	}
	for _, cand := range best {
		kind := classify(ec.Levels, cur, cand)
		fixed := fixedBy(ec, cur, cand)
		eol, _ := support(ec.Lines, lineKey(ec.Levels, cand), now, ad.EOLWarn)
		out.Targets = append(out.Targets, Target{
			Version: cand.tagWithSuffix(cur.suffix), Kind: kind, Security: len(fixed) > 0, Advisories: fixed,
			EOL: eol, Automatic: kind != KindMajor && kindRank(kind) <= kindRank(ec.AutoMax),
		})
	}
	sort.Slice(out.Targets, func(i, j int) bool {
		a, b := out.Targets[i], out.Targets[j]
		if kindRank(a.Kind) != kindRank(b.Kind) {
			return kindRank(a.Kind) < kindRank(b.Kind)
		}
		va, _ := parseVersion(a.Version)
		vb, _ := parseVersion(b.Version)
		return compareVersions(va, vb) > 0
	})
	return out
}

// BestAutomatic picks the newest target the policy level and the engine both allow.
func (a Advice) BestAutomatic(level string) (Target, bool) {
	var pick Target
	found := false
	for _, t := range a.Targets {
		if !t.Automatic || kindRank(t.Kind) > kindRank(level) {
			continue
		}
		tv, _ := parseVersion(t.Version)
		pv, _ := parseVersion(pick.Version)
		if !found || compareVersions(tv, pv) > 0 {
			pick, found = t, true
		}
	}
	return pick, found
}

// FindTarget returns the advised target with that exact version.
func (a Advice) FindTarget(v string) (Target, bool) {
	for _, t := range a.Targets {
		if t.Version == v {
			return t, true
		}
	}
	return Target{}, false
}

func parseAll(raw []string) []version {
	out := make([]version, 0, len(raw))
	for _, r := range raw {
		if v, ok := parseVersion(r); ok {
			out = append(out, v)
		}
	}
	return out
}

func support(lines []SupportLine, line string, now time.Time, warn time.Duration) (string, string) {
	for _, l := range lines {
		if l.Line != line {
			continue
		}
		eol, err := time.Parse(time.DateOnly, l.EOL)
		if err != nil {
			return l.EOL, SupportUnknown
		}
		switch {
		case !now.Before(eol):
			return l.EOL, SupportEOL
		case eol.Sub(now) <= warn:
			return l.EOL, SupportEOLSoon
		default:
			return l.EOL, SupportSupported
		}
	}
	return "", SupportUnknown
}

// affecting lists the advisory ids that apply to v: some fixed version of
// v's own line is newer than v.
func affecting(ec EngineCatalog, v version) []string {
	var ids []string
	for _, adv := range ec.Advisories {
		if advisoryApplies(ec.Levels, adv, v) {
			ids = append(ids, adv.IDs...)
		}
	}
	return ids
}

func advisoryApplies(levels []string, adv Advisory, v version) bool {
	line := seriesKey(levels, v)
	for _, f := range adv.Fixed {
		fv, ok := parseVersion(f)
		if ok && seriesKey(levels, fv) == line && compareVersions(v, fv) < 0 {
			return true
		}
	}
	return false
}

// fixedBy lists ids that affect from but not to. A target on another line
// only counts when that line has a fixed version at or below it.
func fixedBy(ec EngineCatalog, from, to version) []string {
	var ids []string
	for _, adv := range ec.Advisories {
		if !advisoryApplies(ec.Levels, adv, from) || advisoryApplies(ec.Levels, adv, to) {
			continue
		}
		if !lineCovered(ec.Levels, adv, to) {
			continue
		}
		ids = append(ids, adv.IDs...)
	}
	return ids
}

func lineCovered(levels []string, adv Advisory, v version) bool {
	line := seriesKey(levels, v)
	for _, f := range adv.Fixed {
		if fv, ok := parseVersion(f); ok && seriesKey(levels, fv) == line {
			return true
		}
	}
	return false
}

package dbcatalogbuild

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// TagLister lists every tag of a registry repository.
type TagLister interface {
	ListTags(ctx context.Context, repository string) ([]string, error)
}

// CycleSource returns an engine's release cycles from endoflife.date.
type CycleSource interface {
	Cycles(ctx context.Context, slug string) ([]Cycle, error)
}

// Builder assembles catalog engines. Accept, when set, drops versions the
// control plane would refuse (ValidateEngineVersion).
type Builder struct {
	Tags   TagLister
	Cycles CycleSource
	Accept func(engine, version string) bool
	Now    func() time.Time
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now().UTC()
	}
	return time.Now().UTC()
}

// BuildEngine builds one engine from its spec.
func (b *Builder) BuildEngine(ctx context.Context, s Spec) (Engine, error) {
	tags, err := b.Tags.ListTags(ctx, s.Repo)
	if err != nil {
		return Engine{}, fmt.Errorf("dbcatalogbuild: %s tags: %w", s.ID, err)
	}
	cycles := map[string]Cycle{}
	if s.EOLSlug != "" {
		list, err := b.Cycles.Cycles(ctx, s.EOLSlug)
		if err != nil {
			return Engine{}, fmt.Errorf("dbcatalogbuild: %s lifecycle: %w", s.ID, err)
		}
		for _, c := range list {
			cycles[c.Cycle] = c
		}
	}
	e := Engine{Source: Source{
		TagsURL: DockerHubBase + "/v2/" + s.Repo + "/tags/list", ReleaseNotesURL: s.ReleaseNotesURL,
	}}
	if s.EOLSlug != "" {
		e.Source.EOLURL = "https://endoflife.date/" + s.EOLSlug
	}
	stable := newestPerLine(tags, s.StableRe, s.Lines)
	newestLine := ""
	for _, line := range s.Lines {
		tag, ok := stable[line]
		if !ok || (b.Accept != nil && !b.Accept(s.ID, tag)) {
			continue
		}
		c := cycles[line]
		eol := c.eolDate()
		ch := ChannelStable
		switch {
		case eol != "" && eol <= b.now().Format("2006-01-02"):
			ch = ChannelEOL
		case c.LTS:
			ch = ChannelLTS
		}
		v := Version{Version: tag, Tag: tag, Channel: ch, EOL: eol}
		if c.Latest == tag {
			v.Released = c.LatestReleaseDate
		}
		e.Versions = append(e.Versions, v)
		e.Lines = append(e.Lines, Line{Line: line, EOL: eol, LTS: c.LTS})
		if ch != ChannelEOL && (newestLine == "" || cmpNums(line, newestLine) > 0) {
			newestLine = line
		}
	}
	for i := range e.Versions {
		e.Versions[i].Recommended = newestLine != "" && stable[newestLine] == e.Versions[i].Tag
	}
	if s.PreRe != nil {
		e.Versions = append(e.Versions, b.prerelease(s, tags, stable)...)
	}
	return e, nil
}

func (b *Builder) prerelease(s Spec, tags []string, stable map[string]string) []Version {
	top := ""
	for line := range stable {
		if top == "" || cmpNums(line, top) > 0 {
			top = line
		}
	}
	best := map[string]string{}
	for _, t := range tags {
		m := s.PreRe.FindStringSubmatch(t)
		if m == nil || (top != "" && cmpNums(m[1], top) <= 0) {
			continue
		}
		if cur, ok := best[m[1]]; !ok || cmpNums(t, cur) > 0 {
			best[m[1]] = t
		}
	}
	var out []Version
	for _, t := range best {
		if b.Accept != nil && !b.Accept(s.ID, t) {
			continue
		}
		out = append(out, Version{Version: t, Tag: t, Channel: preChannel(t)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

func preChannel(tag string) string {
	if strings.Contains(tag, "rc") {
		return ChannelRC
	}
	return ChannelBeta
}

// Build builds every spec. One failing engine fails the run: a partial
// catalog must never replace a complete one.
func (b *Builder) Build(ctx context.Context, specs []Spec) (File, error) {
	f := File{Schema: SchemaVersion, GeneratedAt: b.now().Format(time.RFC3339), Engines: map[string]Engine{}}
	var errs []error
	for _, s := range specs {
		e, err := b.BuildEngine(ctx, s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.Engines[s.ID] = e
	}
	return f, errors.Join(errs...)
}

func newestPerLine(tags []string, re *regexp.Regexp, lines []string) map[string]string {
	allowed := map[string]bool{}
	for _, l := range lines {
		allowed[l] = true
	}
	out := map[string]string{}
	for _, t := range tags {
		m := re.FindStringSubmatch(t)
		if m == nil || !allowed[m[1]] {
			continue
		}
		if cur, ok := out[m[1]]; !ok || cmpNums(t, cur) > 0 {
			out[m[1]] = t
		}
	}
	return out
}

// cmpNums compares the leading dotted numbers of a and b.
func cmpNums(a, b string) int {
	pa, pb := nums(a), nums(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
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

func nums(s string) []int {
	var out []int
	cur, in := 0, false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cur, in = cur*10+int(r-'0'), true
		case r == '.' && in:
			out = append(out, cur)
			cur, in = 0, false
		default:
			if in {
				out = append(out, cur)
			}
			return out
		}
	}
	if in {
		out = append(out, cur)
	}
	return out
}

// ValidateAgainst enforces the publish gates: schema, no empty engine, and
// no engine shrinking by more than a third versus prev (nil prev skips it).
func ValidateAgainst(next File, prev *File) error {
	if next.Schema != SchemaVersion {
		return fmt.Errorf("dbcatalogbuild: schema %d, want %d", next.Schema, SchemaVersion)
	}
	if len(next.Engines) == 0 {
		return errors.New("dbcatalogbuild: catalog has no engines")
	}
	var errs []error
	for id, e := range next.Engines {
		if len(e.Versions) == 0 {
			errs = append(errs, fmt.Errorf("engine %q has no versions", id))
			continue
		}
		if prev == nil {
			continue
		}
		old := len(prev.Engines[id].Versions)
		if old > 0 && len(e.Versions)*3 < old*2 {
			errs = append(errs, fmt.Errorf("engine %q shrank from %d to %d versions", id, old, len(e.Versions)))
		}
	}
	sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
	return errors.Join(errs...)
}

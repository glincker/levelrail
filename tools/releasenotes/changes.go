package main

import (
	"regexp"
	"strconv"
	"strings"
)

// Change is one merged commit (normally one squashed PR) in a release.
type Change struct {
	Type         string
	Scope        string
	Subject      string
	SHA          string
	PR           int
	Author       string
	Labels       []string
	Breaking     bool
	BreakingNote string
	// ReleaseNote is the author-written sentence from the PR body, if any.
	ReleaseNote string
	// NoteNone marks a PR whose author wrote NONE as its release note.
	NoteNone bool
}

// Section is a group of changes rendered under one heading.
type Section struct {
	Title   string
	Changes []Change
}

var (
	subjectRe  = regexp.MustCompile(`^([a-z]+)(?:\(([^)]*)\))?(!)?:\s*(.+)$`)
	prSuffixRe = regexp.MustCompile(`\s*\(#(\d+)\)\s*$`)
	breakingRe = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:\s*(.+(?:\n[^\n#].*)*)`)
	headingRe  = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*\s*$`)
	fenceRe    = regexp.MustCompile("(?s)```release-note\\s*\\n(.*?)```")
	commentRe  = regexp.MustCompile(`(?s)<!--.*?-->`)
	securityRe = regexp.MustCompile(`(?i)\b(security|cve-\d|ghsa-|vulnerab)`)
)

// ParseCommit parses a conventional-commit message into a Change.
func ParseCommit(sha, message string) Change {
	subject, body, _ := strings.Cut(strings.TrimSpace(message), "\n")
	c := Change{SHA: sha, Type: "other", Subject: strings.TrimSpace(subject)}
	if m := prSuffixRe.FindStringSubmatch(c.Subject); m != nil {
		c.PR, _ = strconv.Atoi(m[1])
		c.Subject = strings.TrimSpace(prSuffixRe.ReplaceAllString(c.Subject, ""))
	}
	if m := subjectRe.FindStringSubmatch(c.Subject); m != nil {
		c.Type, c.Scope, c.Subject = m[1], m[2], m[4]
		c.Breaking = m[3] == "!"
	}
	if note := BreakingNote(body); note != "" {
		c.Breaking, c.BreakingNote = true, note
	}
	return c
}

// BreakingNote returns the text of a BREAKING CHANGE footer, if present.
func BreakingNote(text string) string {
	m := breakingRe.FindStringSubmatch(commentRe.ReplaceAllString(text, ""))
	if m == nil {
		return ""
	}
	return strings.Join(strings.Fields(m[1]), " ")
}

// ExtractReleaseNote reads the "Release note" section (or a release-note
// fenced block) of a PR body. none is true when the author wrote NONE.
func ExtractReleaseNote(body string) (note string, none bool) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if m := fenceRe.FindStringSubmatch(body); m != nil {
		note = m[1]
	} else {
		note = sectionAfterHeading(body, "release note")
	}
	note = strings.TrimSpace(commentRe.ReplaceAllString(note, ""))
	note, _, _ = strings.Cut(note, "\n\n")
	if breakingRe.MatchString(note) {
		note = ""
	}
	note = strings.Join(strings.Fields(note), " ")
	if strings.EqualFold(strings.Trim(note, ".` "), "none") {
		return "", true
	}
	return note, false
}

func sectionAfterHeading(body, title string) string {
	locs := headingRe.FindAllStringSubmatchIndex(body, -1)
	for i, loc := range locs {
		if !strings.EqualFold(strings.TrimSpace(body[loc[2]:loc[3]]), title) {
			continue
		}
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return body[loc[1]:end]
	}
	return ""
}

// ApplyPR merges PR metadata (body, labels, author) into the change.
func (c *Change) ApplyPR(body, author string, labels []string) {
	c.Labels = labels
	if author != "" {
		c.Author = author
	}
	c.ReleaseNote, c.NoteNone = ExtractReleaseNote(body)
	if note := BreakingNote(body); note != "" {
		c.Breaking = true
		if c.BreakingNote == "" {
			c.BreakingNote = note
		}
	}
	if c.HasLabel("breaking-change") {
		c.Breaking = true
	}
}

// HasLabel reports whether the change's PR carries the label.
func (c Change) HasLabel(name string) bool {
	for _, l := range c.Labels {
		if strings.EqualFold(l, name) {
			return true
		}
	}
	return false
}

// Text is the user-facing line: the PR's release note, else the subject.
func (c Change) Text() string {
	if c.ReleaseNote != "" {
		return strings.TrimSuffix(c.ReleaseNote, ".")
	}
	return c.Subject
}

func (c Change) isSecurity() bool {
	return c.Type == "security" || c.HasLabel("type/security") ||
		(c.Type == "fix" && securityRe.MatchString(c.Subject+" "+c.ReleaseNote))
}

var visibleSections = []struct {
	title string
	match func(Change) bool
}{
	{"Features", func(c Change) bool { return c.Type == "feat" }},
	{"Security", Change.isSecurity},
	{"Bug fixes", func(c Change) bool { return c.Type == "fix" }},
	{"Performance", func(c Change) bool { return c.Type == "perf" }},
	{"Documentation", func(c Change) bool { return c.Type == "docs" }},
}

// Group sorts changes into the visible sections, in display order, and
// returns everything else (chores, CI, tests, refactors, NONE notes) as
// the collapsed maintenance list. Release-please's own release commits
// are dropped entirely.
func Group(changes []Change) (sections []Section, maintenance []Change) {
	buckets := make([][]Change, len(visibleSections))
	for _, c := range changes {
		if c.Type == "chore" && strings.HasPrefix(c.Subject, "release ") {
			continue
		}
		placed := false
		if !c.NoteNone {
			for i, s := range visibleSections {
				if s.match(c) {
					buckets[i] = append(buckets[i], c)
					placed = true
					break
				}
			}
		}
		if !placed {
			maintenance = append(maintenance, c)
		}
	}
	for i, s := range visibleSections {
		if len(buckets[i]) > 0 {
			sections = append(sections, Section{Title: s.title, Changes: buckets[i]})
		}
	}
	return sections, maintenance
}

// Highlights returns PRs labeled "highlight", else the first max features.
func Highlights(changes []Change, max int) []Change {
	var labeled, feats []Change
	for _, c := range changes {
		if c.NoteNone {
			continue
		}
		if c.HasLabel("highlight") {
			labeled = append(labeled, c)
		} else if c.Type == "feat" {
			feats = append(feats, c)
		}
	}
	if len(labeled) > 0 {
		return labeled
	}
	if len(feats) > max {
		feats = feats[:max]
	}
	return feats
}

// Breaking returns the changes that carry a breaking-change marker.
func Breaking(changes []Change) []Change {
	var out []Change
	for _, c := range changes {
		if c.Breaking {
			out = append(out, c)
		}
	}
	return out
}

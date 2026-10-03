// Package changelog parses the repo's release-please-maintained
// CHANGELOG.md into the entries GET /api/v1/changelog and the
// dashboard's "What's new" panel serve, so release notes keep exactly
// one source of truth instead of a second, hand-curated copy.
package changelog

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Entry is one released version's notes.
type Entry struct {
	Version string
	Date    string
	Bullets []string
}

var (
	// Matches both release-please's linked heading
	// ("## [0.2.0-beta.14](url) (2026-09-23)") and a plain one
	// ("## 0.1.0 (2026-09-09)").
	versionHeaderRe = regexp.MustCompile(`^## (?:\[([^\]]+)\]\([^)]*\)|(\S+)) \((\d{4}-\d{2}-\d{2})\)\s*$`)
	bulletRe        = regexp.MustCompile(`^[*-] (.+)$`)
	// Strips release-please's trailing "([#123](url)) ([sha](url))"
	// reference links so a bullet reads as plain prose in the panel.
	trailingLinkRe = regexp.MustCompile(`\s*\(\[[^\]]+\]\([^()]*\)\)`)
)

// Parse reads CHANGELOG.md's "## version (date)" headings and the
// "* bullet" / "- bullet" lines under each one, across every
// "### Section" subheading, into a flat, newest-first Bullets list. Any
// other "## " heading (for example a hand-written "Unreleased" preamble
// sometimes left at the file's tail) resets the current entry so its
// bullets are never misattributed to the previous release.
func Parse(markdown string) []Entry {
	var entries []Entry
	idx := -1
	for _, line := range strings.Split(markdown, "\n") {
		if m := versionHeaderRe.FindStringSubmatch(line); m != nil {
			version := m[1]
			if version == "" {
				version = m[2]
			}
			entries = append(entries, Entry{Version: version, Date: m[3]})
			idx = len(entries) - 1
			continue
		}
		if strings.HasPrefix(line, "## ") {
			idx = -1
			continue
		}
		if idx < 0 {
			continue
		}
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			bullet := strings.TrimSpace(trailingLinkRe.ReplaceAllString(m[1], ""))
			if bullet != "" {
				entries[idx].Bullets = append(entries[idx].Bullets, bullet)
			}
		}
	}
	return entries
}

// Latest returns at most n entries from the front of entries, which are
// already newest-first (CHANGELOG.md's own order). n <= 0 returns every
// entry.
func Latest(entries []Entry, n int) []Entry {
	if n <= 0 || n >= len(entries) {
		return entries
	}
	return entries[:n]
}

// ReadFile returns the contents of the first existing path in
// candidates (empty strings are skipped), with found=false and no error
// when none exist: a missing CHANGELOG.md is a normal, non-fatal state
// for a bare binary install that hasn't shipped one yet, not an error.
func ReadFile(candidates ...string) (content string, found bool, err error) {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		data, readErr := os.ReadFile(p) //nolint:gosec // operator-controlled file path (env var or a fixed default), not user input
		if readErr == nil {
			return string(data), true, nil
		}
		if !os.IsNotExist(readErr) {
			return "", false, fmt.Errorf("changelog: read %s: %w", p, readErr)
		}
	}
	return "", false, nil
}

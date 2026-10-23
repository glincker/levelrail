// Package branchdeploy holds the pure rules for branch deploys: which branches
// qualify and what stable name a branch gets.
package branchdeploy

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Modes for an app's branch deploy setting.
const (
	ModeOff      = "off"
	ModeAll      = "all"
	ModePatterns = "patterns"
)

// Settings is an app's branch deploy policy. Patterns are exact branch names
// or globs where "*" matches any run of characters, including "/".
type Settings struct {
	Mode     string
	Patterns []string
}

// ValidMode reports whether m is a known mode.
func ValidMode(m string) bool {
	return m == ModeOff || m == ModeAll || m == ModePatterns
}

// Eligible reports whether branch gets a branch deploy. The production branch
// is never eligible: it already deploys as the app itself.
func (s Settings) Eligible(branch, productionBranch string) bool {
	if branch == "" || branch == productionBranch {
		return false
	}
	switch s.Mode {
	case ModeAll:
		return true
	case ModePatterns:
		for _, p := range s.Patterns {
			if MatchGlob(p, branch) {
				return true
			}
		}
	}
	return false
}

// MatchGlob reports whether name matches pattern, where "*" matches any run
// of characters (including "/") and every other character matches itself.
func MatchGlob(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	p, n := 0, 0
	star, mark := -1, 0
	for n < len(name) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, n
			p++
		case p < len(pattern) && pattern[p] == name[n]:
			p++
			n++
		case star >= 0:
			p = star + 1
			mark++
			n = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

const (
	hashLen = 6
	// minSlugLen keeps room for a readable stem plus the hash suffix.
	minSlugLen  = 10
	dnsLabelMax = 63
	nameInfix   = "-br-"
)

// Slug turns a branch name into a DNS-safe slug of at most maxLen characters.
// A branch that is already lowercase alphanumerics and dashes keeps its name;
// any other branch gets a short hash of the original so "feature/x" and
// "feature-x" never share a URL. The result is deterministic.
func Slug(branch string, maxLen int) string {
	if maxLen < minSlugLen {
		maxLen = minSlugLen
	}
	clean := sanitize(branch)
	if clean != "" && clean == branch && len(clean) <= maxLen {
		return clean
	}
	if clean == "" {
		clean = "branch"
	}
	sum := sha256.Sum256([]byte(branch))
	suffix := "-" + hex.EncodeToString(sum[:])[:hashLen]
	stem := clean
	if limit := maxLen - len(suffix); len(stem) > limit {
		stem = strings.Trim(stem[:limit], "-")
	}
	if stem == "" {
		stem = "branch"
	}
	return stem + suffix
}

func sanitize(branch string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(branch) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// MaxSlugLen is the longest slug that keeps "<app>-br-<slug>" within one DNS
// label.
func MaxSlugLen(appName string) int {
	n := dnsLabelMax - len(appName) - len(nameInfix)
	if n < minSlugLen {
		return minSlugLen
	}
	return n
}

// AppName is the name a branch deploy runs under.
func AppName(appName, slug string) string {
	return appName + nameInfix + slug
}

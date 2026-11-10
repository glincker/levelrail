package spec

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Ref kinds ShortRefName reports for a full git ref.
const (
	RefKindBranch = "branch"
	RefKindTag    = "tag"
)

// RefPatternLimits bounds a pattern list; callers seed it from runtime settings.
type RefPatternLimits struct {
	MaxPatterns   int
	MaxPatternLen int
	MaxListBytes  int
}

// ParseRefPatterns splits a comma or newline separated list, trimming blanks.
func ParseRefPatterns(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// NormalizeRefPatterns returns the canonical newline joined form that is stored.
func NormalizeRefPatterns(raw string) string {
	return strings.Join(ParseRefPatterns(raw), "\n")
}

// ValidateRefPatterns rejects a list the matcher would not accept, so a bad pattern fails at save time.
func ValidateRefPatterns(raw string, lim RefPatternLimits) error {
	patterns := ParseRefPatterns(raw)
	if total := len(strings.Join(patterns, "")); lim.MaxListBytes > 0 && total > lim.MaxListBytes {
		return fmt.Errorf("the patterns total %d bytes, at most %d are allowed per list", total, lim.MaxListBytes)
	}
	if lim.MaxPatterns > 0 && len(patterns) > lim.MaxPatterns {
		return fmt.Errorf("at most %d patterns per list, got %d", lim.MaxPatterns, len(patterns))
	}
	for _, p := range patterns {
		if err := validateRefPattern(p, lim.MaxPatternLen); err != nil {
			return fmt.Errorf("pattern %q: %w", truncateForError(p), err)
		}
	}
	return nil
}

func validateRefPattern(p string, maxLen int) error {
	if maxLen > 0 && len(p) > maxLen {
		return fmt.Errorf("longer than %d bytes", maxLen)
	}
	body := strings.TrimPrefix(p, "!")
	if body == "" {
		return errors.New("a ! negation needs a pattern after it")
	}
	if strings.HasPrefix(body, "refs/") {
		return errors.New("patterns match the short name, drop the refs/heads/ or refs/tags/ prefix")
	}
	if strings.Contains(body, "***") {
		return errors.New("*** is not a valid wildcard, use * or **")
	}
	for _, r := range body {
		if !refPatternRuneAllowed(r) {
			return fmt.Errorf("character %q is not allowed (letters, digits and - _ . / + @ * ? only)", r)
		}
	}
	return nil
}

func refPatternRuneAllowed(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("-_./+@*?", r)
}

// ShortRefName strips refs/heads/ or refs/tags/ and reports the kind. Any other
// ref (refs/pull, notes, a hostile value) returns ok false and never matches.
func ShortRefName(ref string, maxBytes int) (name, kind string, ok bool) {
	if maxBytes > 0 && len(ref) > maxBytes {
		return "", "", false
	}
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		name, kind = strings.TrimPrefix(ref, "refs/heads/"), RefKindBranch
	case strings.HasPrefix(ref, "refs/tags/"):
		name, kind = strings.TrimPrefix(ref, "refs/tags/"), RefKindTag
	default:
		return "", "", false
	}
	if name == "" || !utf8.ValidString(name) {
		return "", "", false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", "", false
		}
	}
	return name, kind, true
}

// MatchRefPatterns reports whether name passes the list. A ! pattern wins over
// any positive one whatever the order; a list of only negations allows the rest.
func MatchRefPatterns(raw, name string) bool {
	patterns := ParseRefPatterns(raw)
	if len(patterns) == 0 {
		return true
	}
	positive, sawPositive := false, false
	for _, p := range patterns {
		if neg, ok := strings.CutPrefix(p, "!"); ok {
			if globMatch(neg, name) {
				return false
			}
			continue
		}
		sawPositive = true
		if !positive && globMatch(p, name) {
			positive = true
		}
	}
	return positive || !sawPositive
}

// globMatch runs * (no slash), ** (any) and ? (one character) as a set of live
// pattern positions, so cost is len(pattern) times len(name) with no backtracking.
func globMatch(pattern, name string) bool {
	toks := compileGlob(pattern)
	cur := make([]bool, len(toks)+1)
	next := make([]bool, len(toks)+1)
	cur[0] = true
	closeStars(toks, cur)
	for _, c := range name {
		clear(next)
		for j, t := range toks {
			if !cur[j] {
				continue
			}
			switch {
			case t.star:
				if t.slash || c != '/' {
					next[j] = true
				}
			case t.any:
				if c != '/' {
					next[j+1] = true
				}
			case t.lit == c:
				next[j+1] = true
			}
		}
		closeStars(toks, next)
		cur, next = next, cur
	}
	return cur[len(toks)]
}

type globTok struct {
	lit   rune
	any   bool
	star  bool
	slash bool
}

func compileGlob(p string) []globTok {
	toks := make([]globTok, 0, len(p))
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				toks = append(toks, globTok{star: true, slash: true})
				i++
				continue
			}
			toks = append(toks, globTok{star: true})
		case '?':
			toks = append(toks, globTok{any: true})
		default:
			toks = append(toks, globTok{lit: rune(p[i])})
		}
	}
	return toks
}

// closeStars lets a live star position also skip itself, since a star may match nothing.
func closeStars(toks []globTok, live []bool) {
	for j, t := range toks {
		if live[j] && t.star {
			live[j+1] = true
		}
	}
}

func truncateForError(s string) string {
	const keep = 64
	if len(s) <= keep {
		return s
	}
	return s[:keep] + "..."
}

// Package pathfilter matches changed file paths against doublestar-style
// globs, the filter behind pipeline `paths` / `paths_ignore` and the
// deploy-on-push path filters.
package pathfilter

import (
	"errors"
	"fmt"
	"strings"
)

// Filter is a set of include globs and ignore globs. An empty Paths list
// includes every path; a path matching any Ignore glob is dropped.
type Filter struct {
	Paths  []string
	Ignore []string
}

// IsZero reports whether the filter has no globs at all.
func (f Filter) IsZero() bool { return len(f.Paths) == 0 && len(f.Ignore) == 0 }

// Validate reports the first malformed glob.
func (f Filter) Validate() error {
	all := append(append([]string{}, f.Paths...), f.Ignore...)
	for _, g := range all {
		if strings.TrimSpace(g) == "" {
			return errors.New("pathfilter: empty glob")
		}
		if err := check(g); err != nil {
			return fmt.Errorf("pathfilter: glob %q: %w", g, err)
		}
	}
	return nil
}

// Result is the outcome of applying a Filter to a changed-file list.
type Result struct {
	Run    bool
	Reason string
}

// Apply decides whether a change touching files should run. An empty file
// list means the file set is unknown, so the answer is always to run.
func (f Filter) Apply(files []string) Result {
	if f.IsZero() || len(files) == 0 {
		return Result{Run: true}
	}
	for _, file := range files {
		if f.matches(file) {
			return Result{Run: true}
		}
	}
	if len(f.Paths) > 0 && !f.anyIncluded(files) {
		return Result{Reason: "skipped: no changed path matched paths"}
	}
	return Result{Reason: "skipped: every changed path matched paths_ignore"}
}

func (f Filter) anyIncluded(files []string) bool {
	for _, file := range files {
		if matchAny(f.Paths, Normalize(file)) {
			return true
		}
	}
	return false
}

func (f Filter) matches(file string) bool {
	file = Normalize(file)
	if len(f.Paths) > 0 && !matchAny(f.Paths, file) {
		return false
	}
	return !matchAny(f.Ignore, file)
}

// Normalize converts Windows separators to slashes and strips a leading
// "./" or "/".
func Normalize(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	p = strings.TrimPrefix(p, "./")
	return strings.TrimLeft(p, "/")
}

func matchAny(globs []string, file string) bool {
	for _, g := range globs {
		if Match(g, file) {
			return true
		}
	}
	return false
}

// Match reports whether file matches glob. Supported syntax: `*` (any run
// of characters within one segment), `?` (one character within a segment),
// `**` as a whole segment (zero or more segments), `[abc]`, `[a-z]` and
// `[!a]` classes, and `{a,b}` alternation. A glob ending in "/" matches
// everything beneath that directory. Wildcards match dotfiles. Both sides
// are normalized, so backslash separators work.
func Match(glob, file string) bool {
	glob = Normalize(glob)
	file = Normalize(file)
	if glob == "" {
		return false
	}
	if strings.HasSuffix(glob, "/") {
		glob += "**"
	}
	for _, g := range expandBraces(glob) {
		if matchSegments(strings.Split(g, "/"), strings.Split(file, "/")) {
			return true
		}
	}
	return false
}

func check(glob string) error {
	for _, g := range expandBraces(Normalize(glob)) {
		for _, seg := range strings.Split(g, "/") {
			if seg == "**" {
				continue
			}
			if strings.Contains(seg, "**") {
				return errors.New("** must be a whole path segment")
			}
			if strings.Count(seg, "[") != strings.Count(seg, "]") {
				return errors.New("unterminated character class")
			}
		}
	}
	return nil
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 || !matchRunes([]rune(pat[0]), []rune(name[0])) {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

func matchRunes(p, s []rune) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for i := 0; i <= len(s); i++ {
				if matchRunes(p[1:], s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
		case '[':
			if len(s) == 0 {
				return false
			}
			ok, n := matchClass(p, s[0])
			if n == 0 {
				if s[0] != '[' {
					return false
				}
			} else {
				if !ok {
					return false
				}
				p = p[n-1:]
			}
		default:
			if len(s) == 0 || s[0] != p[0] {
				return false
			}
		}
		p, s = p[1:], s[1:]
	}
	return len(s) == 0
}

// matchClass matches r against the class starting at p[0]=='['. It returns
// the number of pattern runes consumed, 0 when the class is unterminated.
func matchClass(p []rune, r rune) (matched bool, consumed int) {
	i := 1
	negate := false
	if i < len(p) && (p[i] == '!' || p[i] == '^') {
		negate = true
		i++
	}
	first := true
	for ; i < len(p); i++ {
		if p[i] == ']' && !first {
			return matched != negate, i + 1
		}
		first = false
		lo := p[i]
		if i+2 < len(p) && p[i+1] == '-' && p[i+2] != ']' {
			if r >= lo && r <= p[i+2] {
				matched = true
			}
			i += 2
			continue
		}
		if r == lo {
			matched = true
		}
	}
	return false, 0
}

// expandBraces expands {a,b,c} groups recursively.
func expandBraces(g string) []string {
	start := strings.IndexByte(g, '{')
	if start < 0 {
		return []string{g}
	}
	depth, end, last := 0, -1, start+1
	var parts []string
	for i := start; i < len(g) && end < 0; i++ {
		switch g[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				parts = append(parts, g[last:i])
				end = i
			}
		case ',':
			if depth == 1 {
				parts = append(parts, g[last:i])
				last = i + 1
			}
		}
	}
	if end < 0 {
		return []string{g}
	}
	var out []string
	for _, p := range parts {
		out = append(out, expandBraces(g[:start]+p+g[end+1:])...)
	}
	return out
}

package repopreview

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	htmlTag   = regexp.MustCompile(`<[^>]*>`)
	mdImage   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMarkers = regexp.MustCompile("(?m)^[#>\\-*\\s]+|[`*_~|]")
	spaces    = regexp.MustCompile(`\s+`)
)

// Excerpt turns README markdown into plain text of at most limit runes. The
// result is data only and is never meant to be rendered as HTML.
func Excerpt(md string, limit int) string {
	s := htmlTag.ReplaceAllString(md, " ")
	s = mdImage.ReplaceAllString(s, " ")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdMarkers.ReplaceAllString(s, " ")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, s)
	return clip(strings.TrimSpace(spaces.ReplaceAllString(s, " ")), limit)
}

func clip(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if limit <= 0 || len(r) <= limit {
		return string(r)
	}
	return strings.TrimSpace(string(r[:limit])) + "..."
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func percentages(in map[string]int64) []Language {
	var total int64
	for _, n := range in {
		total += n
	}
	if total == 0 {
		return nil
	}
	out := make([]Language, 0, len(in))
	for name, n := range in {
		out = append(out, Language{Name: name, Percent: float64(n) * 100 / float64(total)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Percent != out[j].Percent {
			return out[i].Percent > out[j].Percent
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

package search

import (
	"sort"
	"strings"
)

// Span is a half-open range of Unicode code points in a hit's title.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// highlightSpans marks where terms match title. A term with no substring
// match (a typo found by the trigram fallback) marks its shared 3-rune windows.
func highlightSpans(title string, terms []string) []Span {
	if len(terms) == 0 {
		return nil
	}
	hay := []rune(strings.ToLower(title))
	var spans []Span
	for _, t := range terms {
		needle := []rune(t)
		if len(needle) == 0 {
			continue
		}
		found := false
		for i := 0; i+len(needle) <= len(hay); i++ {
			if runesEqual(hay[i:i+len(needle)], needle) {
				spans = append(spans, Span{i, i + len(needle)})
				found = true
				i += len(needle) - 1
			}
		}
		if found || len(needle) < 3 {
			continue
		}
		for j := 0; j+3 <= len(needle); j++ {
			for i := 0; i+3 <= len(hay); i++ {
				if runesEqual(hay[i:i+3], needle[j:j+3]) {
					spans = append(spans, Span{i, i + 3})
				}
			}
		}
	}
	return mergeSpans(spans)
}

func runesEqual(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mergeSpans(in []Span) []Span {
	if len(in) == 0 {
		return nil
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Start < in[j].Start })
	out := []Span{in[0]}
	for _, s := range in[1:] {
		last := &out[len(out)-1]
		if s.Start <= last.End {
			last.End = max(last.End, s.End)
			continue
		}
		out = append(out, s)
	}
	return out
}

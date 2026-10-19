package telemetry

import (
	"math"
	"sort"
	"strings"
)

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// RankLogsByRelevance orders entries, which already matched query, by BM25
// score (best first), newest first among equal scores. Terms under NOT or
// -word never score. The corpus is the matched set, so scores compare lines
// within one result, not across searches.
func RankLogsByRelevance(entries []LogEntry, query string) []LogEntry {
	terms := scoringTerms(query)
	out := make([]LogEntry, len(entries))
	copy(out, entries)
	if len(terms) == 0 || len(out) < 2 {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
		return out
	}

	lowered := make([]string, len(out))
	lengths := make([]float64, len(out))
	var totalLen float64
	for i, e := range out {
		lowered[i] = strings.ToLower(e.Message)
		lengths[i] = float64(len(strings.Fields(lowered[i])))
		totalLen += lengths[i]
	}
	avgLen := totalLen / float64(len(out))
	if avgLen == 0 {
		avgLen = 1
	}

	n := float64(len(out))
	idf := make([]float64, len(terms))
	for ti, term := range terms {
		df := 0.0
		for _, m := range lowered {
			if strings.Contains(m, term) {
				df++
			}
		}
		idf[ti] = math.Log(1 + (n-df+0.5)/(df+0.5))
	}

	scores := make([]float64, len(out))
	for i, m := range lowered {
		for ti, term := range terms {
			tf := float64(strings.Count(m, term))
			if tf == 0 {
				continue
			}
			norm := tf + bm25K1*(1-bm25B+bm25B*lengths[i]/avgLen)
			scores[i] += idf[ti] * tf * (bm25K1 + 1) / norm
		}
	}

	idx := make([]int, len(out))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if scores[idx[a]] != scores[idx[b]] {
			return scores[idx[a]] > scores[idx[b]]
		}
		return out[idx[a]].Timestamp.After(out[idx[b]].Timestamp)
	})
	ranked := make([]LogEntry, len(out))
	for i, k := range idx {
		ranked[i] = out[k]
	}
	return ranked
}

func scoringTerms(query string) []string {
	toks, err := lexLogSearch(strings.TrimSpace(query))
	if err != nil {
		return nil
	}
	var terms []string
	prevNot := false
	for _, t := range toks {
		switch t.kind {
		case tokTerm:
			if !prevNot && t.value != "" {
				terms = append(terms, strings.ToLower(t.value))
			}
			prevNot = false
		case tokOp:
			prevNot = t.value == "NOT"
		default:
			prevNot = false
		}
	}
	return terms
}

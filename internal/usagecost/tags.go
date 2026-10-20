package usagecost

import "sort"

// UntaggedKey groups resources that carry no tag.
const UntaggedKey = "untagged"

// RollupTags ranks cost by tag. tagsOf maps "kind:name" to tag names. A resource
// with several tags counts in each, so tag totals can exceed the report total.
func RollupTags(p, prevP Period, rows, prev []DailyCost, tagsOf map[string][]string) []Group {
	n := p.Days()
	dayIndex := make(map[string]int, n)
	for i := 0; i < n; i++ {
		dayIndex[DayKey(p.From.AddDate(0, 0, i))] = i
	}
	keysFor := func(row DailyCost) []string {
		if t := tagsOf[row.Kind+":"+row.Name]; len(t) > 0 {
			return t
		}
		return []string{UntaggedKey}
	}
	acc := map[string]*Group{}
	for _, row := range rows {
		i, ok := dayIndex[row.Day]
		if !ok || (row.Kind != KindApp && row.Kind != KindDatabase) {
			continue
		}
		for _, k := range keysFor(row) {
			add(acc, k, row, n, i, func(g *Group) { g.ProjectID, g.TeamID, g.EnvironmentID = "", "", "" })
		}
	}
	prevTotal := map[string]float64{}
	for _, row := range prev {
		d, err := ParseDay(row.Day)
		if err != nil || d.Before(prevP.From) || !d.Before(prevP.To) || (row.Kind != KindApp && row.Kind != KindDatabase) {
			continue
		}
		for _, k := range keysFor(row) {
			prevTotal[k] += row.Total()
		}
	}
	out := make([]Group, 0, len(acc))
	for k, g := range acc {
		g.Previous = prevTotal[k]
		g.Delta = g.Total - g.Previous
		g.DeltaPct = pct(g.Total, g.Previous)
		out = append(out, *g)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Total != out[b].Total {
			return out[a].Total > out[b].Total
		}
		return out[a].Key < out[b].Key
	})
	return out
}

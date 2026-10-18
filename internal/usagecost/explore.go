package usagecost

import (
	"math"
	"sort"
)

// Explorer group dimensions.
const (
	GroupApp     = "app"
	GroupProject = "project"
	GroupTag     = "tag"
	GroupNode    = "node"
	GroupKind    = "kind"
)

// OtherKey collapses the groups beyond the top series of an exploration.
const OtherKey = "other"

const (
	exploreTopSeries = 10
	exploreMovers    = 5
	monthKeyLen      = 7
	bandZ            = 1.2816
	thinBandPct      = 0.25
)

// Forecast confidence labels.
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// ValidGroup reports whether g is a dimension Explore understands.
func ValidGroup(g string) bool {
	switch g {
	case GroupApp, GroupProject, GroupTag, GroupNode, GroupKind:
		return true
	}
	return false
}

// ExploreInput selects what Explore aggregates.
type ExploreInput struct {
	Period   Period
	Previous Period
	Rows     []DailyCost
	Prev     []DailyCost
	GroupBy  string
	// Monthly buckets by calendar month instead of by day.
	Monthly bool
	// TagsOf maps "kind:name" to tag names, used by the tag dimension.
	TagsOf map[string][]string
}

// Series is one group's cost over the buckets.
type Series struct {
	Key      string    `json:"key"`
	Total    float64   `json:"total"`
	Previous float64   `json:"previous"`
	Delta    float64   `json:"delta"`
	DeltaPct *float64  `json:"delta_pct,omitempty"`
	Values   []float64 `json:"values"`
}

// Exploration is the result of Explore.
type Exploration struct {
	GroupBy  string    `json:"group_by"`
	Buckets  []string  `json:"buckets"`
	Trend    []float64 `json:"trend"`
	Total    float64   `json:"total"`
	Previous float64   `json:"previous"`
	DeltaPct *float64  `json:"delta_pct,omitempty"`
	Series   []Series  `json:"series"`
	// Movers are the groups whose cost grew most against the previous period.
	Movers []Series `json:"movers"`
}

func bucketKey(day string, monthly bool) string {
	if monthly && len(day) >= monthKeyLen {
		return day[:monthKeyLen]
	}
	return day
}

// Buckets lists the day or month keys covered by p, oldest first.
func Buckets(p Period, monthly bool) []string {
	var out []string
	if monthly {
		for d := StartOfMonth(p.From); d.Before(p.To); d = d.AddDate(0, 1, 0) {
			out = append(out, DayKey(d)[:monthKeyLen])
		}
		return out
	}
	for d := p.From; d.Before(p.To); d = d.AddDate(0, 0, 1) {
		out = append(out, DayKey(d))
	}
	return out
}

func groupKeys(row DailyCost, group string, tagsOf map[string][]string) []string {
	switch group {
	case GroupApp:
		if row.Kind == KindApp {
			return []string{row.Name}
		}
	case GroupKind:
		if row.Kind == KindApp || row.Kind == KindDatabase {
			return []string{row.Kind}
		}
	case GroupProject:
		if row.Kind == KindApp || row.Kind == KindDatabase {
			return []string{orUnassigned(row.ProjectID)}
		}
	case GroupNode:
		if row.Kind == KindApp || row.Kind == KindDatabase {
			return []string{orUnassigned(row.NodeID)}
		}
	case GroupTag:
		if row.Kind == KindApp || row.Kind == KindDatabase {
			if t := tagsOf[row.Kind+":"+row.Name]; len(t) > 0 {
				return t
			}
			return []string{UntaggedKey}
		}
	}
	return nil
}

// Explore aggregates cost per bucket and group, ranks the groups, and finds
// the largest increases against the previous period.
func Explore(in ExploreInput) Exploration {
	buckets := Buckets(in.Period, in.Monthly)
	index := make(map[string]int, len(buckets))
	for i, b := range buckets {
		index[b] = i
	}
	out := Exploration{GroupBy: in.GroupBy, Buckets: buckets, Trend: make([]float64, len(buckets)), Series: []Series{}, Movers: []Series{}}
	acc := map[string]*Series{}
	for _, row := range in.Rows {
		i, ok := index[bucketKey(row.Day, in.Monthly)]
		if !ok {
			continue
		}
		keys := groupKeys(row, in.GroupBy, in.TagsOf)
		if len(keys) == 0 {
			continue
		}
		cost := row.Total()
		out.Trend[i] += cost
		out.Total += cost
		for _, k := range keys {
			s := acc[k]
			if s == nil {
				s = &Series{Key: k, Values: make([]float64, len(buckets))}
				acc[k] = s
			}
			s.Values[i] += cost
			s.Total += cost
		}
	}
	for _, row := range in.Prev {
		d, err := ParseDay(row.Day)
		if err != nil || d.Before(in.Previous.From) || !d.Before(in.Previous.To) {
			continue
		}
		keys := groupKeys(row, in.GroupBy, in.TagsOf)
		if len(keys) == 0 {
			continue
		}
		cost := row.Total()
		out.Previous += cost
		for _, k := range keys {
			s := acc[k]
			if s == nil {
				s = &Series{Key: k, Values: make([]float64, len(buckets))}
				acc[k] = s
			}
			s.Previous += cost
		}
	}
	out.DeltaPct = pct(out.Total, out.Previous)
	all := make([]Series, 0, len(acc))
	for _, s := range acc {
		s.Delta = s.Total - s.Previous
		s.DeltaPct = pct(s.Total, s.Previous)
		all = append(all, *s)
	}
	sort.Slice(all, func(a, b int) bool {
		if all[a].Total != all[b].Total {
			return all[a].Total > all[b].Total
		}
		return all[a].Key < all[b].Key
	})
	out.Movers = increases(all)
	out.Series = topSeries(all, len(buckets))
	return out
}

func increases(all []Series) []Series {
	cp := make([]Series, 0, len(all))
	for _, s := range all {
		if s.Delta > 0 {
			cp = append(cp, s)
		}
	}
	sort.Slice(cp, func(a, b int) bool {
		if cp[a].Delta != cp[b].Delta {
			return cp[a].Delta > cp[b].Delta
		}
		return cp[a].Key < cp[b].Key
	})
	if len(cp) > exploreMovers {
		cp = cp[:exploreMovers]
	}
	return cp
}

func topSeries(all []Series, n int) []Series {
	var live []Series
	for _, s := range all {
		if s.Total > 0 || s.Previous > 0 {
			live = append(live, s)
		}
	}
	if len(live) <= exploreTopSeries {
		return append([]Series{}, live...)
	}
	out := append([]Series{}, live[:exploreTopSeries]...)
	rest := Series{Key: OtherKey, Values: make([]float64, n)}
	for _, s := range live[exploreTopSeries:] {
		rest.Total += s.Total
		rest.Previous += s.Previous
		for i, v := range s.Values {
			rest.Values[i] += v
		}
	}
	rest.Delta = rest.Total - rest.Previous
	rest.DeltaPct = pct(rest.Total, rest.Previous)
	return append(out, rest)
}

// Band is a month end forecast with an 80 percent range.
type Band struct {
	Spent      float64 `json:"spent"`
	Mid        float64 `json:"mid"`
	Low        float64 `json:"low"`
	High       float64 `json:"high"`
	Confidence string  `json:"confidence"`
	Samples    int     `json:"samples"`
}

// ForecastBand projects the month end like ForecastMonthEnd and adds a range
// from the day to day spread: the remaining days are treated as independent
// draws, so the band widens with the square root of the days left. With fewer
// than three full days it falls back to a fixed relative band and reports low
// confidence, because a spread over one or two samples means nothing.
func ForecastBand(spent, elapsedDays float64, daysInMonth int, recentDaily []float64) Band {
	mid := ForecastMonthEnd(spent, elapsedDays, daysInMonth, recentDaily)
	b := Band{Spent: spent, Mid: mid, Low: mid, High: mid, Confidence: ConfidenceLow, Samples: len(recentDaily)}
	remaining := float64(daysInMonth) - elapsedDays
	if remaining <= 0 {
		b.Confidence = ConfidenceHigh
		return b
	}
	projected := mid - spent
	if len(recentDaily) < 3 {
		b.Low, b.High = spent+projected*(1-thinBandPct), spent+projected*(1+thinBandPct)
		return b
	}
	var mean float64
	for _, v := range recentDaily {
		mean += v
	}
	mean /= float64(len(recentDaily))
	var ss float64
	for _, v := range recentDaily {
		ss += (v - mean) * (v - mean)
	}
	sd := math.Sqrt(ss / float64(len(recentDaily)-1))
	margin := bandZ * sd * math.Sqrt(remaining)
	b.Low = math.Max(spent, mid-margin)
	b.High = mid + margin
	cv := 0.0
	if mean > 0 {
		cv = sd / mean
	}
	switch {
	case len(recentDaily) >= 5 && cv <= 0.2:
		b.Confidence = ConfidenceHigh
	case cv <= 0.5:
		b.Confidence = ConfidenceMedium
	}
	return b
}

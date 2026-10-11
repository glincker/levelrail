package usagecost

import (
	"math"
	"sort"
	"time"
)

// Group dimensions a report rolls cost up by.
const (
	DimResource    = "resource"
	DimProject     = "project"
	DimEnvironment = "environment"
	DimTeam        = "team"
)

// UnassignedKey groups resources with no project, environment or team.
const UnassignedKey = "unassigned"

// Group is one row of a ranked breakdown.
type Group struct {
	Key           string    `json:"key"`
	Kind          string    `json:"kind,omitempty"`
	Name          string    `json:"name,omitempty"`
	Total         float64   `json:"total"`
	Compute       float64   `json:"compute"`
	Storage       float64   `json:"storage"`
	Egress        float64   `json:"egress"`
	Idle          float64   `json:"idle_share"`
	Previous      float64   `json:"previous"`
	Delta         float64   `json:"delta"`
	DeltaPct      *float64  `json:"delta_pct,omitempty"`
	ProjectID     string    `json:"project_id,omitempty"`
	TeamID        string    `json:"team_id,omitempty"`
	EnvironmentID string    `json:"environment_id,omitempty"`
	Daily         []float64 `json:"daily"`
}

// Totals summarises a report.
type Totals struct {
	Total       float64  `json:"total"`
	Allocated   float64  `json:"allocated"`
	Unallocated float64  `json:"unallocated"`
	Previous    float64  `json:"previous"`
	DeltaPct    *float64 `json:"delta_pct,omitempty"`
}

// Report is the full showback result for one period.
type Report struct {
	Period       Period   `json:"period"`
	Previous     Period   `json:"previous"`
	Days         []string `json:"days"`
	Totals       Totals   `json:"totals"`
	Resources    []Group  `json:"resources"`
	Projects     []Group  `json:"projects"`
	Environments []Group  `json:"environments"`
	Teams        []Group  `json:"teams"`
	Movers       []Group  `json:"movers"`
	// Forecast is the projected month end total, set for periods that
	// include the current month's elapsed days.
	Forecast *float64 `json:"forecast,omitempty"`
}

const topMovers = 5

// PreviousPeriod returns the comparison window: for month to date, the same
// number of days at the start of last month; otherwise the window of equal
// length immediately before.
func PreviousPeriod(p Period, monthToDate bool) Period {
	if monthToDate {
		start := StartOfMonth(p.From).AddDate(0, -1, 0)
		return Period{From: start, To: start.AddDate(0, 0, p.Days())}
	}
	n := p.Days()
	return Period{From: p.From.AddDate(0, 0, -n), To: p.From}
}

// Rollup builds a report. rows and prev are allocated daily rows of the
// period and its comparison window; includeIdle controls whether the
// unallocated remainder is shown (callers hide it from team scoped users).
func Rollup(p, prevP Period, rows, prev []DailyCost, includeIdle bool) Report {
	n := p.Days()
	days := make([]string, n)
	for i := range days {
		days[i] = DayKey(p.From.AddDate(0, 0, i))
	}
	dayIndex := make(map[string]int, n)
	for i, d := range days {
		dayIndex[d] = i
	}
	r := Report{Period: p, Previous: prevP, Days: days}
	acc := map[string]map[string]*Group{
		DimResource: {}, DimProject: {}, DimEnvironment: {}, DimTeam: {},
	}
	for _, row := range rows {
		i, ok := dayIndex[row.Day]
		if !ok || row.Kind == KindDay {
			continue
		}
		total := row.Total()
		if row.Kind == KindIdle {
			if includeIdle {
				r.Totals.Unallocated += total
			}
			continue
		}
		r.Totals.Allocated += total
		add(acc[DimResource], row.Kind+":"+row.Name, row, n, i, func(g *Group) { g.Kind, g.Name = row.Kind, row.Name })
		add(acc[DimProject], orUnassigned(row.ProjectID), row, n, i, nil)
		add(acc[DimEnvironment], orUnassigned(row.EnvironmentID), row, n, i, func(g *Group) { g.ProjectID = row.ProjectID })
		add(acc[DimTeam], orUnassigned(row.TeamID), row, n, i, nil)
	}
	r.Totals.Total = r.Totals.Allocated + r.Totals.Unallocated
	prevByDim := previousTotals(prev, prevP, includeIdle)
	r.Totals.Previous = prevByDim.total
	r.Totals.DeltaPct = pct(r.Totals.Total, prevByDim.total)
	finish := func(dim string) []Group {
		out := make([]Group, 0, len(acc[dim]))
		for k, g := range acc[dim] {
			g.Previous = prevByDim.groups[dim][k]
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
	r.Resources, r.Projects = finish(DimResource), finish(DimProject)
	r.Environments, r.Teams = finish(DimEnvironment), finish(DimTeam)
	r.Movers = movers(r.Resources)
	return r
}

func add(m map[string]*Group, key string, row DailyCost, n, day int, init func(*Group)) {
	g := m[key]
	if g == nil {
		g = &Group{Key: key, Daily: make([]float64, n), ProjectID: row.ProjectID, TeamID: row.TeamID, EnvironmentID: row.EnvironmentID}
		if init != nil {
			init(g)
		}
		m[key] = g
	}
	g.Compute += row.ComputeCost
	g.Storage += row.StorageCost
	g.Egress += row.EgressCost
	g.Idle += row.IdleCost
	g.Total += row.Total()
	g.Daily[day] += row.Total()
}

type prevTotals struct {
	total  float64
	groups map[string]map[string]float64
}

func previousTotals(prev []DailyCost, p Period, includeIdle bool) prevTotals {
	out := prevTotals{groups: map[string]map[string]float64{
		DimResource: {}, DimProject: {}, DimEnvironment: {}, DimTeam: {},
	}}
	for _, row := range prev {
		if row.Kind == KindDay {
			continue
		}
		d, err := ParseDay(row.Day)
		if err != nil || d.Before(p.From) || !d.Before(p.To) {
			continue
		}
		t := row.Total()
		if row.Kind == KindIdle {
			if includeIdle {
				out.total += t
			}
			continue
		}
		out.total += t
		out.groups[DimResource][row.Kind+":"+row.Name] += t
		out.groups[DimProject][orUnassigned(row.ProjectID)] += t
		out.groups[DimEnvironment][orUnassigned(row.EnvironmentID)] += t
		out.groups[DimTeam][orUnassigned(row.TeamID)] += t
	}
	return out
}

func orUnassigned(s string) string {
	if s == "" {
		return UnassignedKey
	}
	return s
}

func pct(cur, prev float64) *float64 {
	if prev <= 0 {
		return nil
	}
	v := (cur - prev) / prev * 100
	return &v
}

func movers(resources []Group) []Group {
	cp := append([]Group(nil), resources...)
	sort.Slice(cp, func(a, b int) bool {
		da, db := math.Abs(cp[a].Delta), math.Abs(cp[b].Delta)
		if da != db {
			return da > db
		}
		return cp[a].Key < cp[b].Key
	})
	out := make([]Group, 0, topMovers)
	for _, g := range cp {
		if len(out) == topMovers || g.Delta == 0 {
			break
		}
		out = append(out, g)
	}
	return out
}

// ForecastMonthEnd projects the month's total from spend so far. It uses the
// mean of the most recent full days when available, so a spike at the start
// of the month does not dominate, and falls back to the overall average.
func ForecastMonthEnd(spent, elapsedDays float64, daysInMonth int, recentDaily []float64) float64 {
	remaining := float64(daysInMonth) - elapsedDays
	if remaining <= 0 {
		return spent
	}
	var rate float64
	if len(recentDaily) > 0 {
		for _, v := range recentDaily {
			rate += v
		}
		rate /= float64(len(recentDaily))
	} else if elapsedDays > 0 {
		rate = spent / elapsedDays
	}
	return spent + rate*remaining
}

// ElapsedDays is how many days of the month containing now have passed.
func ElapsedDays(now time.Time) float64 {
	n := now.UTC()
	return float64(n.Day()-1) + float64(n.Hour()*3600+n.Minute()*60+n.Second())/secondsPerDay
}

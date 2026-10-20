package usagecost

import (
	"math"
	"testing"
	"time"
)

func TestForecastBand(t *testing.T) {
	cases := []struct {
		name      string
		spent     float64
		elapsed   float64
		dim       int
		recent    []float64
		wantConf  string
		wantMid   float64
		wantWidth bool
		wantTight bool
		wantThin  bool
		wantExact bool
	}{
		{name: "steady spend is a tight high confidence band", spent: 50, elapsed: 5, dim: 30,
			recent: []float64{10, 10, 10, 10, 10}, wantConf: ConfidenceHigh, wantMid: 300, wantTight: true},
		{name: "noisy spend widens the band", spent: 50, elapsed: 5, dim: 30,
			recent: []float64{2, 25, 4, 30, 8}, wantConf: ConfidenceLow, wantMid: 50 + 13.8*25, wantWidth: true},
		{name: "too few samples uses a fixed thin band", spent: 20, elapsed: 2, dim: 30,
			recent: []float64{10, 10}, wantConf: ConfidenceLow, wantMid: 300, wantThin: true},
		{name: "month over is exact", spent: 90, elapsed: 30, dim: 30,
			recent: []float64{3, 3, 3}, wantConf: ConfidenceHigh, wantMid: 90, wantExact: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := ForecastBand(c.spent, c.elapsed, c.dim, c.recent)
			if b.Confidence != c.wantConf {
				t.Fatalf("confidence = %s, want %s", b.Confidence, c.wantConf)
			}
			if math.Abs(b.Mid-c.wantMid) > 0.5 {
				t.Fatalf("mid = %.2f, want %.2f", b.Mid, c.wantMid)
			}
			if b.Low < c.spent-1e-9 || b.Low > b.Mid+1e-9 || b.High < b.Mid-1e-9 {
				t.Fatalf("band not ordered: %+v", b)
			}
			switch {
			case c.wantTight && b.High-b.Low > 1e-6:
				t.Fatalf("expected zero width, got %+v", b)
			case c.wantWidth && b.High-b.Low < 50:
				t.Fatalf("expected a wide band, got %+v", b)
			case c.wantThin && math.Abs((b.High-b.Low)-0.5*(b.Mid-b.Spent)) > 1e-6:
				t.Fatalf("expected a 25%% band each side, got %+v", b)
			case c.wantExact && (b.Low != b.Mid || b.High != b.Mid):
				t.Fatalf("expected exact, got %+v", b)
			}
		})
	}
}

func day(s string) time.Time {
	d, err := ParseDay(s)
	if err != nil {
		panic(err)
	}
	return d
}

func row(d, kind, name, project, node string, cost float64) DailyCost {
	return DailyCost{DailyUsage: DailyUsage{Day: d, Kind: kind, Name: name, ProjectID: project, NodeID: node}, ComputeCost: cost}
}

func TestExploreGroupsAndMovers(t *testing.T) {
	p := Period{From: day("2026-10-01"), To: day("2026-10-04")}
	prevP := Period{From: day("2026-09-28"), To: day("2026-10-01")}
	rows := []DailyCost{
		row("2026-10-01", KindApp, "web", "p1", "n1", 4),
		row("2026-10-02", KindApp, "web", "p1", "n1", 6),
		row("2026-10-02", KindApp, "api", "p2", "n2", 3),
		row("2026-10-03", KindDatabase, "db", "p1", "n1", 2),
		row("2026-10-03", KindIdle, "idle", "", "n1", 99),
	}
	prev := []DailyCost{
		row("2026-09-29", KindApp, "web", "p1", "n1", 5),
		row("2026-09-30", KindApp, "api", "p2", "n2", 5),
	}
	tags := map[string][]string{"app:web": {"prod"}, "app:api": {"prod", "edge"}}
	cases := []struct {
		group     string
		wantKeys  []string
		wantTotal float64
	}{
		{GroupApp, []string{"web", "api"}, 13},
		{GroupKind, []string{"app", "database"}, 15},
		{GroupProject, []string{"p1", "p2"}, 15},
		{GroupNode, []string{"n1", "n2"}, 15},
		{GroupTag, []string{"prod", "untagged", "edge"}, 15},
	}
	for _, c := range cases {
		t.Run(c.group, func(t *testing.T) {
			e := Explore(ExploreInput{Period: p, Previous: prevP, Rows: rows, Prev: prev, GroupBy: c.group, TagsOf: tags})
			if e.Total != c.wantTotal {
				t.Fatalf("total = %v, want %v", e.Total, c.wantTotal)
			}
			if len(e.Buckets) != 3 || len(e.Trend) != 3 {
				t.Fatalf("buckets = %v", e.Buckets)
			}
			var got []string
			for _, s := range e.Series {
				got = append(got, s.Key)
			}
			if len(got) != len(c.wantKeys) {
				t.Fatalf("series = %v, want %v", got, c.wantKeys)
			}
		})
	}
	e := Explore(ExploreInput{Period: p, Previous: prevP, Rows: rows, Prev: prev, GroupBy: GroupApp})
	if len(e.Movers) != 1 || e.Movers[0].Key != "web" || e.Movers[0].Delta != 5 {
		t.Fatalf("movers = %+v, want only web up by 5 (api fell)", e.Movers)
	}
}

func TestExploreMonthlyBucketsAndOther(t *testing.T) {
	p := Period{From: day("2026-08-15"), To: day("2026-10-02")}
	var rows []DailyCost
	for i := 0; i < 12; i++ {
		rows = append(rows, row("2026-09-10", KindApp, string(rune('a'+i)), "", "", float64(i+1)))
	}
	e := Explore(ExploreInput{Period: p, Previous: Period{From: day("2026-06-01"), To: day("2026-08-01")}, Rows: rows, GroupBy: GroupApp, Monthly: true})
	if len(e.Buckets) != 3 || e.Buckets[0] != "2026-08" || e.Buckets[2] != "2026-10" {
		t.Fatalf("buckets = %v", e.Buckets)
	}
	if len(e.Series) != 11 || e.Series[10].Key != OtherKey || e.Series[10].Values[1] != 3 {
		t.Fatalf("expected top 10 plus other (a+b), got %d series", len(e.Series))
	}
}

package usagecost

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"
)

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func usage(name, node string, cores float64, memGiB float64) DailyUsage {
	return DailyUsage{
		Day: "2026-10-10", Kind: KindApp, Name: name, NodeID: node, ProjectID: "p-" + name,
		UpSeconds: secondsPerDay, CPUAvgCores: cores, CPUP50Cores: cores, MemAvgBytes: memGiB * gib, MemP50Bytes: memGiB * gib,
	}
}

func sumCost(rows []DailyCost) (alloc, idle float64) {
	for _, r := range rows {
		if r.Kind == KindIdle {
			idle += r.Total()
		} else {
			alloc += r.Total()
		}
	}
	return alloc, idle
}

func TestAllocateDayNodeMode(t *testing.T) {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	nodes := map[string]NodePrice{"n1": {NodeID: "n1", MonthlyPrice: 310, VCPUs: 4, MemoryGiB: 8}}
	tests := []struct {
		name      string
		settings  Settings
		usage     []DailyUsage
		wantAlloc float64
		wantIdle  float64
		check     func(*testing.T, DayResult)
	}{
		{
			name:     "two apps share proportionally, remainder is idle",
			settings: Settings{Mode: ModeNode, CPUWeightPct: 50},
			usage:    []DailyUsage{usage("a", "n1", 1, 2), usage("b", "n1", 0.5, 1)},
			// daily 10; a share = .5*1/4+.5*2/8 = .25 ; b = .125
			wantAlloc: 3.75, wantIdle: 6.25,
			check: func(t *testing.T, r DayResult) {
				near(t, "a", r.Rows[0].ComputeCost, 2.5)
				near(t, "b", r.Rows[1].ComputeCost, 1.25)
			},
		},
		{
			name:      "idle share fully spread allocates the whole bill",
			settings:  Settings{Mode: ModeNode, CPUWeightPct: 50, IdleSpreadPct: 100},
			usage:     []DailyUsage{usage("a", "n1", 1, 2), usage("b", "n1", 0.5, 1)},
			wantAlloc: 10, wantIdle: 0,
			check: func(t *testing.T, r DayResult) {
				near(t, "a", r.Rows[0].Total(), 20.0/3)
				near(t, "b", r.Rows[1].Total(), 10.0/3)
			},
		},
		{
			name:      "half spread",
			settings:  Settings{Mode: ModeNode, CPUWeightPct: 50, IdleSpreadPct: 50},
			usage:     []DailyUsage{usage("a", "n1", 1, 2)},
			wantAlloc: 2.5 + 3.75, wantIdle: 3.75,
		},
		{
			name:      "zero usage leaves the whole node idle",
			settings:  Settings{Mode: ModeNode, IdleSpreadPct: 100},
			usage:     []DailyUsage{usage("a", "n1", 0, 0)},
			wantAlloc: 0, wantIdle: 10,
		},
		{
			name:      "overcommit scales down to the bill",
			settings:  Settings{Mode: ModeNode, CPUWeightPct: 50},
			usage:     []DailyUsage{usage("a", "n1", 8, 16), usage("b", "n1", 4, 8)},
			wantAlloc: 10, wantIdle: 0,
			check: func(t *testing.T, r DayResult) {
				near(t, "ratio", r.Rows[0].Total()/r.Rows[1].Total(), 2)
			},
		},
		{
			name:     "p50 basis",
			settings: Settings{Mode: ModeNode, CPUWeightPct: 50, Basis: BasisP50},
			usage: func() []DailyUsage {
				u := usage("a", "n1", 4, 8)
				u.CPUP50Cores, u.MemP50Bytes = 1, 2*gib
				return []DailyUsage{u}
			}(),
			wantAlloc: 2.5, wantIdle: 7.5,
		},
		{
			name:     "partial uptime counts only while running",
			settings: Settings{Mode: ModeNode, CPUWeightPct: 50},
			usage: func() []DailyUsage {
				u := usage("a", "n1", 1, 2)
				u.UpSeconds = secondsPerDay / 2
				return []DailyUsage{u}
			}(),
			wantAlloc: 1.25, wantIdle: 8.75,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := AllocateDay(DayInput{Day: day, Settings: tt.settings, Nodes: nodes, Usage: tt.usage, Fraction: 1})
			alloc, idle := sumCost(res.Rows)
			near(t, "allocated", alloc, tt.wantAlloc)
			near(t, "idle", idle, tt.wantIdle)
			if tt.check != nil {
				tt.check(t, res)
			}
		})
	}
}

func TestAllocateDayUnpricedNodeAndPartialDay(t *testing.T) {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	nodes := map[string]NodePrice{"n1": {NodeID: "n1", MonthlyPrice: 310, VCPUs: 4, MemoryGiB: 8}}
	res := AllocateDay(DayInput{
		Day: day, Settings: Settings{Mode: ModeNode}, Nodes: nodes, Fraction: 0.5,
		Usage: []DailyUsage{usage("a", "n1", 1, 2), usage("far", "n2", 1, 1)},
	})
	if len(res.Unpriced) != 1 || res.Unpriced[0] != "n2" {
		t.Fatalf("unpriced = %v, want [n2]", res.Unpriced)
	}
	alloc, idle := sumCost(res.Rows)
	near(t, "allocated", alloc+idle, 5)
}

func TestAllocateDayUnitMode(t *testing.T) {
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	u := usage("a", "", 2, 4)
	u.StorageBytes = 10 * gib
	u.EgressBytes = 5 * gib
	res := AllocateDay(DayInput{
		Day: day, Fraction: 1, Usage: []DailyUsage{u},
		Settings: Settings{Mode: ModeUnit, CPUCoreHour: 0.02, MemGiBHour: 0.005, StorageGiBMonth: 0.31, EgressGiB: 0.01, IdleSpreadPct: 10},
	})
	r := res.Rows[0]
	near(t, "compute", r.ComputeCost, 2*24*0.02+4*24*0.005)
	near(t, "storage", r.StorageCost, 10*0.31/31)
	near(t, "egress", r.EgressCost, 0.05)
	near(t, "overhead", r.IdleCost, r.ComputeCost*0.1)
}

func TestPriceChangeMidMonthKeepsHistory(t *testing.T) {
	period := Period{From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	var rows []DailyCost
	for i, price := range []float64{0.02, 0.04} {
		d := period.From.AddDate(0, 0, i)
		u := usage("a", "", 1, 0)
		u.Day = DayKey(d)
		res := AllocateDay(DayInput{Day: d, Fraction: 1, Usage: []DailyUsage{u}, Settings: Settings{CPUCoreHour: price}})
		rows = append(rows, res.Rows...)
	}
	r := Rollup(period, PreviousPeriod(period, false), rows, nil, true)
	near(t, "total", r.Totals.Total, 24*0.02+24*0.04)
	near(t, "day0", r.Resources[0].Daily[0], 24*0.02)
	near(t, "day1", r.Resources[0].Daily[1], 24*0.04)
}

func TestIntegrate(t *testing.T) {
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	var s []Sample
	for i := 0; i < 120; i++ {
		s = append(s, Sample{At: from.Add(time.Duration(i) * 15 * time.Second), Value: 50})
	}
	got := Integrate(s, from, to)
	near(t, "avg", got.Avg, 50)
	near(t, "p50", got.P50, 50)
	near(t, "up", got.UpSeconds, 1800)

	if z := Integrate(nil, from, to); z.UpSeconds != 0 || z.Avg != 0 {
		t.Errorf("empty window = %+v", z)
	}
}

func TestIntegrateGapIsNotAveraged(t *testing.T) {
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	s := []Sample{
		{At: from, Value: 100}, {At: from.Add(15 * time.Second), Value: 100},
		{At: from.Add(30 * time.Second), Value: 100}, {At: from.Add(time.Hour), Value: 100},
	}
	got := Integrate(s, from, from.Add(2*time.Hour))
	if got.UpSeconds > 120 {
		t.Errorf("a one hour gap accrued uptime: %v", got.UpSeconds)
	}
}

func TestRollupTrendMoversAndIdle(t *testing.T) {
	period := Period{From: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	prevP := PreviousPeriod(period, true)
	if prevP.From.Month() != time.September || prevP.Days() != 2 {
		t.Fatalf("previous = %+v", prevP)
	}
	mk := func(day, name, project string, cost float64) DailyCost {
		return DailyCost{DailyUsage: DailyUsage{Day: day, Kind: KindApp, Name: name, ProjectID: project, TeamID: "t1"}, ComputeCost: cost}
	}
	rows := []DailyCost{
		mk("2026-10-01", "a", "p1", 2), mk("2026-10-02", "a", "p1", 4),
		mk("2026-10-01", "b", "p2", 1),
		{DailyUsage: DailyUsage{Day: "2026-10-01", Kind: KindIdle, Name: "n1"}, IdleCost: 3},
	}
	prev := []DailyCost{mk("2026-09-01", "a", "p1", 2), mk("2026-09-02", "b", "p2", 3)}
	r := Rollup(period, prevP, rows, prev, true)
	near(t, "total", r.Totals.Total, 10)
	near(t, "unallocated", r.Totals.Unallocated, 3)
	near(t, "previous", r.Totals.Previous, 5)
	if r.Projects[0].Key != "p1" || r.Projects[0].Total != 6 {
		t.Errorf("projects = %+v", r.Projects)
	}
	if r.Movers[0].Name != "a" && r.Movers[0].Name != "b" {
		t.Errorf("movers = %+v", r.Movers)
	}
	near(t, "a delta", r.Resources[0].Delta, 4)
	hidden := Rollup(period, prevP, rows, prev, false)
	near(t, "hidden unallocated", hidden.Totals.Unallocated, 0)
	near(t, "hidden total", hidden.Totals.Total, 7)
}

func TestForecastMonthEnd(t *testing.T) {
	tests := []struct {
		name    string
		spent   float64
		elapsed float64
		dim     int
		recent  []float64
		want    float64
	}{
		{"recent run rate", 30, 10, 30, []float64{2, 4}, 30 + 3*20},
		{"falls back to average", 30, 10, 30, nil, 90},
		{"month over", 50, 30, 30, []float64{1}, 50},
		{"nothing elapsed", 0, 0, 30, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			near(t, "forecast", ForecastMonthEnd(tt.spent, tt.elapsed, tt.dim, tt.recent), tt.want)
		})
	}
}

func TestBudgetBreaches(t *testing.T) {
	b := Budget{ID: "b1", Scope: ScopeProject, ScopeID: "p1", Monthly: 100}
	tests := []struct {
		name     string
		spent    float64
		forecast float64
		want     []string
	}{
		{"under", 10, 50, nil},
		{"warn at 80", 80, 90, []string{"b1:warn"}},
		{"over", 100, 120, []string{"b1:over"}},
		{"forecast only", 40, 130, []string{"b1:forecast"}},
		{"warn plus forecast", 85, 130, []string{"b1:warn", "b1:forecast"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keys []string
			for _, br := range NewStatus(b, "web", "USD", tt.spent, tt.forecast).Breaches() {
				keys = append(keys, br.Key)
			}
			if strings.Join(keys, ",") != strings.Join(tt.want, ",") {
				t.Errorf("breaches = %v, want %v", keys, tt.want)
			}
		})
	}
	if got := NewStatus(Budget{ID: "z", Monthly: 0}, "x", "USD", 5, 5).Breaches(); got != nil {
		t.Errorf("zero budget breached: %v", got)
	}
}

func TestWriteCSVNeutralisesFormulas(t *testing.T) {
	r := Report{Resources: []Group{{Kind: KindApp, Name: "=cmd()", Total: 1.5, Compute: 1.5}}, Totals: Totals{Unallocated: 2}}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, "USD", r, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "'=cmd()") || !strings.Contains(out, "unallocated") {
		t.Errorf("csv = %q", out)
	}
}

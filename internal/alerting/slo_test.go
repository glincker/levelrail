package alerting

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// sloFake serves samples by metric and honours the queried time range.
type sloFake struct {
	byMetric map[string][]telemetry.Sample
	queries  int
}

func (f *sloFake) QueryMetrics(_ context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error) {
	f.queries++
	var out []telemetry.Sample
	for _, s := range f.byMetric[metric] {
		if s.ResourceID == resourceID && !s.Timestamp.Before(from) && !s.Timestamp.After(to) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *sloFake) add(app, metric string, at time.Time, v float64) {
	if f.byMetric == nil {
		f.byMetric = map[string][]telemetry.Sample{}
	}
	f.byMetric[metric] = append(f.byMetric[metric], telemetry.Sample{ResourceID: "service:" + app, Metric: metric, Timestamp: at, Value: v})
}

var sloNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBurnRate(t *testing.T) {
	tests := []struct {
		name               string
		bad, total, target float64
		want               float64
	}{
		{"no traffic", 0, 0, 99.9, 0},
		{"no errors", 0, 1000, 99.9, 0},
		{"exactly on budget", 1, 1000, 99.9, 1},
		{"fast burn 14.4x", 14.4, 1000, 99.9, 14.4},
		{"all bad", 1000, 1000, 99.9, 1000},
		{"bad above total is clamped", 5000, 1000, 99.9, 1000},
		{"negative bad ignored", -3, 1000, 99.9, 0},
		{"99 percent target", 5, 100, 99, 5},
		{"impossible 100 percent target never burns", 5, 100, 100, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BurnRate(tc.bad, tc.total, tc.target); !approx(got, tc.want) {
				t.Fatalf("BurnRate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBudgetRemaining(t *testing.T) {
	tests := []struct {
		name               string
		bad, total, target float64
		want               float64
	}{
		{"no traffic leaves the budget whole", 0, 0, 99.9, 1},
		{"no errors", 0, 10000, 99.9, 1},
		{"half spent", 5, 10000, 99.9, 0.5},
		{"exactly spent", 10, 10000, 99.9, 0},
		{"overspent goes negative", 20, 10000, 99.9, -1},
		{"bad above total is clamped", 99999, 10000, 99.9, 1 - 10000/10.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BudgetRemaining(tc.bad, tc.total, tc.target); !approx(got, tc.want) {
				t.Fatalf("BudgetRemaining = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSLOConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  SLOConfig
		ok   bool
	}{
		{"availability", SLOConfig{Objective: SLOAvailability, Target: 99.9}, true},
		{"latency", SLOConfig{Objective: SLOLatency, Target: 99, LatencyMs: 300}, true},
		{"latency needs a threshold", SLOConfig{Objective: SLOLatency, Target: 99}, false},
		{"target 100", SLOConfig{Objective: SLOAvailability, Target: 100}, false},
		{"target too low", SLOConfig{Objective: SLOAvailability, Target: 10}, false},
		{"NaN target", SLOConfig{Objective: SLOAvailability, Target: math.NaN()}, false},
		{"unknown objective", SLOConfig{Objective: "uptime", Target: 99}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestCountRequests(t *testing.T) {
	avail := SLOConfig{Objective: SLOAvailability, Target: 99.9}
	latency := SLOConfig{Objective: SLOLatency, Target: 99, LatencyMs: 100}
	from, to := sloNow.Add(-time.Hour), sloNow

	tests := []struct {
		name      string
		cfg       SLOConfig
		setup     func(f *sloFake)
		wantTotal float64
		wantBad   float64
	}{
		{"no traffic", avail, func(*sloFake) {}, 0, 0},
		{"5xx counted", avail, func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-10*time.Minute), 100)
			f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-10*time.Minute), 4)
		}, 100, 4},
		{"deltas from several ticks add up", avail, func(f *sloFake) {
			for i := 1; i <= 3; i++ {
				f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-time.Duration(i)*time.Minute), 50)
				f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-time.Duration(i)*time.Minute), 1)
			}
		}, 150, 3},
		{"counter reset artefacts (negative, NaN, Inf) count as zero", avail, func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-5*time.Minute), 100)
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-4*time.Minute), -9e15)
			f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-3*time.Minute), math.NaN())
			f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-2*time.Minute), math.Inf(1))
			f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-1*time.Minute), 2)
		}, 100, 2},
		{"5xx above total is clamped", avail, func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-5*time.Minute), 10)
			f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-5*time.Minute), 30)
		}, 10, 10},
		{"other apps are ignored", avail, func(f *sloFake) {
			f.add("api", telemetry.MetricHTTPRequests, sloNow.Add(-5*time.Minute), 999)
		}, 0, 0},
		{"samples outside the window are ignored", avail, func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-2*time.Hour), 999)
		}, 0, 0},
		{"latency: slower than the threshold is bad", latency, func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-5*time.Minute), 100)
			f.add("web", telemetry.LatencyBucketMetric(0), sloNow.Add(-5*time.Minute), 60) // <=5ms
			f.add("web", telemetry.LatencyBucketMetric(4), sloNow.Add(-5*time.Minute), 20) // <=100ms
			f.add("web", telemetry.LatencyBucketMetric(5), sloNow.Add(-5*time.Minute), 15) // <=250ms
			f.add("web", telemetry.LatencyBucketMetric(11), sloNow.Add(-5*time.Minute), 5) // overflow
		}, 100, 20},
		{"latency: threshold between bounds rounds down", SLOConfig{Objective: SLOLatency, Target: 99, LatencyMs: 200}, func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-5*time.Minute), 100)
			f.add("web", telemetry.LatencyBucketMetric(4), sloNow.Add(-5*time.Minute), 80)
			f.add("web", telemetry.LatencyBucketMetric(5), sloNow.Add(-5*time.Minute), 20)
		}, 100, 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &sloFake{}
			tc.setup(f)
			got, err := CountRequests(context.Background(), f, "web", from, to, tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if !approx(got.Total, tc.wantTotal) || !approx(got.Bad, tc.wantBad) {
				t.Fatalf("got %+v, want total=%v bad=%v", got, tc.wantTotal, tc.wantBad)
			}
		})
	}
}

// spread adds total requests and bad 5xx evenly over the last span, one tick a minute.
func spread(f *sloFake, span time.Duration, total, bad float64) {
	n := int(span / time.Minute)
	for i := 0; i < n; i++ {
		at := sloNow.Add(-time.Duration(i) * time.Minute).Add(-time.Second)
		f.add("web", telemetry.MetricHTTPRequests, at, total/float64(n))
		if bad > 0 {
			f.add("web", telemetry.MetricHTTPResponses5xx, at, bad/float64(n))
		}
	}
}

func TestComputeSLOStatusTiers(t *testing.T) {
	cfg := SLOConfig{Objective: SLOAvailability, Target: 99.9}
	p := DefaultSLOPolicy()
	tests := []struct {
		name      string
		setup     func(f *sloFake)
		firing    bool
		page      bool
		wantTiers []string
	}{
		{"no traffic at all", func(*sloFake) {}, false, false, nil},
		{"healthy", func(f *sloFake) { spread(f, 72*time.Hour, 720000, 0) }, false, false, nil},
		{"fast burn on both windows pages", func(f *sloFake) {
			spread(f, 72*time.Hour, 720000, 0)
			spread(f, time.Hour, 60000, 60000*0.02) // 2% bad = 20x
		}, true, true, []string{"page_fast", "page_slow", "ticket_fast", "ticket_slow"},
		},
		{"short window recovered: no page, only the slower ticket tiers", func(f *sloFake) {
			// Errors 30..60m ago only: 1h long window burns hard, the 5m window is clean.
			for i := 30; i < 60; i++ {
				at := sloNow.Add(-time.Duration(i) * time.Minute)
				f.add("web", telemetry.MetricHTTPRequests, at, 1000)
				f.add("web", telemetry.MetricHTTPResponses5xx, at, 100)
			}
			for i := 0; i < 30; i++ {
				f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-time.Duration(i)*time.Minute), 1000)
			}
		}, true, false, []string{"ticket_fast", "ticket_slow"}},
		{"slow steady burn opens a ticket, no page", func(f *sloFake) {
			spread(f, 72*time.Hour, 4320000, 4320000*0.0015) // 1.5x for 3 days
		}, true, false, []string{"ticket_slow"},
		},
		{"too few requests never fires", func(f *sloFake) {
			f.add("web", telemetry.MetricHTTPRequests, sloNow.Add(-time.Minute), 3)
			f.add("web", telemetry.MetricHTTPResponses5xx, sloNow.Add(-time.Minute), 3)
		}, false, false, nil},
		{"partial window: a day-old app is judged on the traffic it has", func(f *sloFake) {
			spread(f, 3*time.Hour, 180000, 180000*0.02)
		}, true, true, []string{"page_fast", "page_slow", "ticket_fast", "ticket_slow"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &sloFake{}
			tc.setup(f)
			st, err := ComputeSLOStatus(context.Background(), f, "web", cfg, p, sloNow)
			if err != nil {
				t.Fatal(err)
			}
			if st.Firing != tc.firing || st.Page != tc.page {
				t.Fatalf("firing=%v page=%v, want %v/%v (tiers %+v)", st.Firing, st.Page, tc.firing, tc.page, st.Tiers)
			}
			var got []string
			for _, ts := range st.Tiers {
				if ts.Firing {
					got = append(got, ts.Name)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.wantTiers, ",") {
				t.Fatalf("firing tiers = %v, want %v", got, tc.wantTiers)
			}
		})
	}
}

func TestComputeSLOStatusBudgetAndWindows(t *testing.T) {
	f := &sloFake{}
	spread(f, 24*time.Hour, 100000, 50) // 0.05% bad against a 0.1% budget
	st, err := ComputeSLOStatus(context.Background(), f, "web", SLOConfig{Objective: SLOAvailability, Target: 99.9}, DefaultSLOPolicy(), sloNow)
	if err != nil {
		t.Fatal(err)
	}
	if !st.HasTraffic || !approx(st.BudgetRemaining, 0.5) {
		t.Fatalf("HasTraffic=%v BudgetRemaining=%v, want true/0.5", st.HasTraffic, st.BudgetRemaining)
	}
	if len(st.Windows) != 7 {
		t.Fatalf("windows = %d, want 7 (5m 30m 1h 2h 6h 1d 3d), got %+v", len(st.Windows), st.Windows)
	}
	for i := 1; i < len(st.Windows); i++ {
		if st.Windows[i-1].Window >= st.Windows[i].Window {
			t.Fatalf("windows not ascending: %+v", st.Windows)
		}
	}
}

func TestComputeSLOStatusNoTrafficBudgetIsWhole(t *testing.T) {
	st, err := ComputeSLOStatus(context.Background(), &sloFake{}, "web", SLOConfig{Objective: SLOAvailability, Target: 99.9}, DefaultSLOPolicy(), sloNow)
	if err != nil {
		t.Fatal(err)
	}
	if st.HasTraffic || st.BudgetRemaining != 1 || st.MaxBurn != 0 || st.Firing {
		t.Fatalf("unexpected status %+v", st)
	}
}

func TestEvaluateSLOBurnStateMachine(t *testing.T) {
	cfg := &SLOConfig{Objective: SLOAvailability, Target: 99.9}
	rule := Rule{ID: "r1", Kind: KindSLOBurn, ResourceID: "service:web", SLO: cfg, Enabled: true}
	p := DefaultSLOPolicy()

	bad := &sloFake{}
	spread(bad, time.Hour, 60000, 1200)
	next, notice, err := EvaluateSLOBurn(context.Background(), bad, rule, p, sloNow)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Firing || next.FiringSince == nil || next.Severity != SeverityCritical {
		t.Fatalf("want firing critical, got %+v", next)
	}
	if !strings.Contains(notice, "page") || !strings.Contains(notice, "99.9% availability") {
		t.Fatalf("notice = %q", notice)
	}
	if next.LastValue == nil || *next.LastValue < 14.4 {
		t.Fatalf("LastValue = %v", next.LastValue)
	}

	healthy := &sloFake{}
	spread(healthy, time.Hour, 60000, 0)
	rule.Firing = true
	got, notice, err := EvaluateSLOBurn(context.Background(), healthy, rule, p, sloNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got.Firing || notice != "" {
		t.Fatalf("want resolved, got firing=%v notice=%q", got.Firing, notice)
	}

	noCfg := Rule{ID: "r2", Kind: KindSLOBurn, ResourceID: "service:web"}
	got, _, err = EvaluateSLOBurn(context.Background(), bad, noCfg, p, sloNow)
	if err != nil || got.Firing {
		t.Fatalf("rule without slo must never fire, got %+v err=%v", got, err)
	}
	if _, _, err := EvaluateSLOBurn(context.Background(), bad, Rule{ID: "r3", Kind: KindSLOBurn, ResourceID: "node:x", SLO: cfg}, p, sloNow); err == nil {
		t.Fatal("non-app resource should error")
	}
}

func TestSLOPolicyFromEnv(t *testing.T) {
	t.Setenv("APP_SLO_PAGE_FAST_FACTOR", "10")
	t.Setenv("APP_SLO_PAGE_FAST_LONG", "2h")
	t.Setenv("APP_SLO_TICKET_SLOW_SHORT", "junk")
	t.Setenv("APP_SLO_MIN_REQUESTS", "50")
	t.Setenv("APP_SLO_WINDOW", "168h")
	p := SLOPolicyFromEnv()
	if p.Tiers[0].Factor != 10 || p.Tiers[0].Long != 2*time.Hour || p.Tiers[0].Short != 5*time.Minute {
		t.Fatalf("tier 0 = %+v", p.Tiers[0])
	}
	if p.Tiers[3].Short != 6*time.Hour {
		t.Fatalf("junk value should keep the default, got %v", p.Tiers[3].Short)
	}
	if p.MinRequests != 50 || p.Window != 168*time.Hour {
		t.Fatalf("policy = %+v", p)
	}
	d := DefaultSLOPolicy()
	if d.Tiers[0].Factor != 14.4 || d.Tiers[1].Factor != 6 || d.Tiers[2].Factor != 3 || d.Tiers[3].Factor != 1 {
		t.Fatalf("defaults drifted: %+v", d.Tiers)
	}
}

func TestSLORuleRoundTripAndEngineTick(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	rule := Rule{ID: "slo1", Name: "web slo", Kind: KindSLOBurn, ResourceID: "service:web", Enabled: true,
		SLO: &SLOConfig{Objective: SLOLatency, Target: 99, LatencyMs: 250}}
	if err := db.SaveRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetRule(ctx, "slo1")
	if err != nil {
		t.Fatal(err)
	}
	if got.SLO == nil || *got.SLO != *rule.SLO {
		t.Fatalf("SLO = %+v, want %+v", got.SLO, rule.SLO)
	}
	plain := Rule{ID: "plain", Name: "p", Kind: KindCrashloop, ResourceID: "service:web", RestartCountThreshold: 3, RestartWindow: time.Minute, Enabled: true}
	if err := db.SaveRule(ctx, plain); err != nil {
		t.Fatal(err)
	}
	if p, _ := db.GetRule(ctx, "plain"); p.SLO != nil {
		t.Fatalf("non-slo rule got SLO %+v", p.SLO)
	}

	live := Rule{ID: "slo2", Name: "live", Kind: KindSLOBurn, ResourceID: "service:web", Enabled: true,
		SLO: &SLOConfig{Objective: SLOAvailability, Target: 99.9}}
	rules := newFakeRuleStore(live)
	f := &sloFake{}
	now := time.Now()
	for i := 0; i < 60; i++ {
		at := now.Add(-time.Duration(i) * time.Minute).Add(-time.Second)
		f.add("web", telemetry.MetricHTTPRequests, at, 1000)
		f.add("web", telemetry.MetricHTTPResponses5xx, at, 100)
	}
	spy := &spyNotifier{}
	engine := newTestEngine(rules, f, nil, nil, spy)
	if err := engine.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	calls := spy.calls()
	if len(calls) != 1 || calls[0].SLONotice == "" || calls[0].Rule.Severity != SeverityCritical {
		t.Fatalf("calls = %+v", calls)
	}
	q := f.queries
	if err := engine.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if f.queries != q {
		t.Fatalf("second tick within the eval interval re-queried metrics (%d -> %d)", q, f.queries)
	}
	if len(spy.calls()) != 1 {
		t.Fatal("a still-firing rule must not notify again")
	}
}

package resourceadvisor

import (
	"testing"
	"time"
)

// screenshotInput mirrors a real server: six unlimited Postgres databases
// of 48 to 217 MiB, one app, and a node that reports no capacity.
func screenshotInput() Input {
	dbs := []struct {
		name, version string
		mem           float64
	}{
		{"askverdict", "17-pgvector", 217 * mib}, {"glinr", "16", 126 * mib}, {"geokit", "16", 71 * mib},
		{"glinrv2", "16", 68 * mib}, {"saasfolk", "16", 67 * mib}, {"n8n", "16", 48 * mib},
	}
	var res []Resource
	for _, d := range dbs {
		res = append(res, Resource{Ref: db(d.name), NodeID: "local", Engine: "postgres", Version: d.version, LinkedApps: 1,
			Memory: series(3*day, 5*time.Minute, 0.05, flat(d.mem)), CPUPercent: series(3*day, 5*time.Minute, 0.3, flat(2))})
	}
	res = append(res, Resource{Ref: app("web"), NodeID: "local", LinkedApps: -1,
		Memory: series(3*day, 5*time.Minute, 0.05, flat(180*mib)), CPUPercent: series(3*day, 5*time.Minute, 0.3, flat(5))})
	return Input{Now: testNow, Resources: res, Nodes: []Node{{ID: "local", NoIdentity: true}}, Thresholds: DefaultThresholds()}
}

func TestAnalyzeScreenshotScenario(t *testing.T) {
	fs := Analyze(screenshotInput())
	count := map[Kind]int{}
	byID := map[string]Finding{}
	for _, f := range fs {
		count[f.Kind]++
		byID[f.ID] = f
	}
	if count[KindMemoryLimitMissing] != 7 {
		t.Fatalf("memory limit suggestions = %d, want 7 (%v)", count[KindMemoryLimitMissing], count)
	}
	if count[KindEngineTuning] != 1 {
		t.Fatalf("tuning findings = %d, want 1 for the biggest", count[KindEngineTuning])
	}
	if _, ok := byID["engine_tuning:database:askverdict"]; !ok {
		t.Fatal("expected tuning for askverdict")
	}
	if count[KindConsolidate] != 1 || count[KindCapacityUnknown] != 1 {
		t.Fatalf("want one consolidation and one capacity note, got %v", count)
	}
	lim := byID["memory_limit_missing:database:askverdict"]
	if lim.Action == nil || lim.Action.Value < 256*mib || lim.Action.Value > 384*mib {
		t.Fatalf("askverdict limit suggestion = %+v", lim.Action)
	}
	if count[KindIdleApp]+count[KindIdleDatabase]+count[KindMemoryGrowth] != 0 {
		t.Fatalf("false positives: %v", count)
	}
}

func TestAnalyzeIsDeterministicAndSorted(t *testing.T) {
	a, b := Analyze(screenshotInput()), Analyze(screenshotInput())
	if len(a) != len(b) {
		t.Fatal("non deterministic length")
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("order differs at %d: %s vs %s", i, a[i].ID, b[i].ID)
		}
		if i > 0 && a[i-1].Severity.Rank() < a[i].Severity.Rank() {
			t.Fatalf("not sorted by severity at %d", i)
		}
	}
}

func TestAnalyzeNotEnoughData(t *testing.T) {
	in := Input{Now: testNow, Resources: []Resource{{Ref: app("new"), Memory: series(3*time.Hour, 5*time.Minute, 0, flat(100*mib))}, {Ref: app("none")}}}
	fs := Analyze(in)
	if len(fs) != 2 {
		t.Fatalf("got %d findings, want 2 not-enough-data notes", len(fs))
	}
	for _, f := range fs {
		if f.Kind != KindNotEnoughData || f.Action != nil {
			t.Fatalf("unexpected %+v", f)
		}
	}
	s := Summarize(fs, 3)
	if s.Total != 0 || s.PendingData != 2 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestAnalyzeSkipsSuspended(t *testing.T) {
	in := Input{Now: testNow, Resources: []Resource{{Ref: app("off"), Suspended: true, Memory: series(week, 5*time.Minute, 0, flat(100*mib))}}}
	if fs := Analyze(in); len(fs) != 0 {
		t.Fatalf("suspended resource produced %v", fs)
	}
}

func TestSummarize(t *testing.T) {
	fs := Analyze(screenshotInput())
	s := Summarize(fs, 2)
	if s.Total == 0 || s.Actionable != 14 {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.TopSavings) != 1 || s.TopSavings[0].Kind != KindConsolidate || s.TopSavings[0].Evidence.Series != nil {
		t.Fatalf("top savings = %+v", s.TopSavings)
	}
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		in      string
		want    Ref
		wantErr bool
	}{
		{"app:web", app("web"), false},
		{"database:main", db("main"), false},
		{"node:n1", Ref{Kind: ResourceNode, Name: "n1"}, false},
		{"pod:x", Ref{}, true},
		{"app:", Ref{}, true},
		{"web", Ref{}, true},
	}
	for _, tc := range tests {
		got, err := ParseRef(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseRef(%q) = %v, %v", tc.in, got, err)
		}
	}
}

func TestThresholdsFromEnv(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	d := DefaultThresholds()
	tests := []struct {
		name    string
		env     map[string]string
		check   func(Thresholds) bool
		wantErr int
	}{
		{"defaults", nil, func(th Thresholds) bool { return th == d }, 0},
		{"days and sizes", map[string]string{"APP_RESOURCE_ADVISOR_WINDOW": "14d", "APP_RESOURCE_ADVISOR_SMALL_DB_MEMORY": "512Mi", "APP_RESOURCE_ADVISOR_MIN_DATA": "48h"},
			func(th Thresholds) bool {
				return th.Window == 14*day && th.SmallDatabaseBytes == 512*mib && th.MinData == 48*time.Hour
			}, 0},
		{"bad values keep defaults", map[string]string{"APP_RESOURCE_ADVISOR_WINDOW": "soon", "APP_RESOURCE_ADVISOR_MEMORY_HEADROOM": "0.5", "APP_RESOURCE_ADVISOR_SLEEP_MINUTES": "1"},
			func(th Thresholds) bool {
				return th.Window == d.Window && th.MemoryHeadroom == d.MemoryHeadroom && th.SleepIdleMinutes == d.SleepIdleMinutes
			}, 3},
		{"inverted ratios fall back", map[string]string{"APP_RESOURCE_ADVISOR_TIGHT_RATIO": "0.3", "APP_RESOURCE_ADVISOR_OVERPROVISION_RATIO": "0.5"},
			func(th Thresholds) bool {
				return th.TightRatio == d.TightRatio && th.OverProvisionRatio == d.OverProvisionRatio
			}, 1},
		{"headroom override", map[string]string{"APP_RESOURCE_ADVISOR_MEMORY_HEADROOM": "1.5"}, func(th Thresholds) bool { return th.MemoryHeadroom == 1.5 }, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th, errs := ThresholdsFromEnv(env(tc.env))
			if len(errs) != tc.wantErr {
				t.Fatalf("errs = %v, want %d", errs, tc.wantErr)
			}
			if !tc.check(th) {
				t.Fatalf("thresholds = %+v", th)
			}
		})
	}
}

func TestHeadroomEnvChangesSuggestion(t *testing.T) {
	r := Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0, flat(200*mib))}
	th := DefaultThresholds()
	base := kinds(limitFindings(r, th, testNow))[KindMemoryLimitMissing].Action.Value
	th.MemoryHeadroom = 2
	wider := kinds(limitFindings(r, th, testNow))[KindMemoryLimitMissing].Action.Value
	if wider <= base {
		t.Fatalf("headroom 2 gave %d, default gave %d", wider, base)
	}
}

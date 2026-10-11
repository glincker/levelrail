package resourceadvisor

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestGrowthFinding(t *testing.T) {
	th := DefaultThresholds()
	linear := func(from, to float64) func(float64) float64 {
		return func(x float64) float64 { return from + (to-from)*x }
	}
	sawtooth := func(x float64) float64 { return 100*mib + 150*mib*math.Mod(x*7, 1) }
	tests := []struct {
		name     string
		res      Resource
		nodeMem  float64
		want     bool
		severity Severity
		eta      bool
	}{
		{"steady leak against a limit", Resource{Ref: app("web"), MemoryLimit: 384 * mib, Memory: series(week, 5*time.Minute, 0.03, linear(100*mib, 300*mib))}, 0, true, SeverityWarn, true},
		{"leak without limit uses node memory", Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0.03, linear(100*mib, 300*mib))}, 64 * gib, true, SeveritySuggest, true},
		{"leak with no ceiling stays info", Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0.03, linear(100*mib, 300*mib))}, 0, true, SeverityInfo, false},
		{"database growth is capped at suggest", Resource{Ref: db("main"), MemoryLimit: 512 * mib, Memory: series(week, 5*time.Minute, 0.03, linear(100*mib, 300*mib))}, 0, true, SeveritySuggest, true},
		{"noisy flat is not growth", Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0.3, flat(200*mib))}, 0, false, "", false},
		{"daily restarts sawtooth is not growth", Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0.02, sawtooth)}, 0, false, "", false},
		{"small growth under the minimum", Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0.01, linear(200*mib, 220*mib))}, 0, false, "", false},
		{"less than a day of data", Resource{Ref: app("web"), Memory: series(12*time.Hour, 5*time.Minute, 0, linear(100*mib, 300*mib))}, 0, false, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := growthFinding(tc.res, tc.nodeMem, th, testNow)
			if ok != tc.want {
				t.Fatalf("found = %v, want %v (%+v)", ok, tc.want, f.Evidence.Numbers)
			}
			if !ok {
				return
			}
			if f.Severity != tc.severity {
				t.Fatalf("severity = %s, want %s", f.Severity, tc.severity)
			}
			if (f.Impact.ExhaustionAt != nil) != tc.eta {
				t.Fatalf("eta present = %v, want %v", f.Impact.ExhaustionAt != nil, tc.eta)
			}
		})
	}
}

func TestIdleFinding(t *testing.T) {
	th := DefaultThresholds()
	mem := series(8*day, 5*time.Minute, 0.05, flat(120*mib))
	quietCPU := series(8*day, 5*time.Minute, 0.5, flat(0.2))
	tests := []struct {
		name string
		res  Resource
		want Kind
	}{
		{"routed app without requests sleeps", Resource{Ref: app("web"), ServesTraffic: true, Memory: mem}, KindIdleApp},
		{"app with requests is busy", Resource{Ref: app("web"), ServesTraffic: true, Memory: mem, Requests: []Sample{{At: testNow.Add(-time.Hour), Value: 3}}}, ""},
		{"app without a domain is not assessed", Resource{Ref: app("worker"), Memory: mem}, ""},
		{"app already sleeping when idle", Resource{Ref: app("web"), ServesTraffic: true, SleepEnabled: true, Memory: mem}, ""},
		{"young app is not idle yet", Resource{Ref: app("web"), ServesTraffic: true, Memory: series(2*day, 5*time.Minute, 0, flat(120*mib))}, ""},
		{"unlinked quiet database", Resource{Ref: db("old"), Memory: mem, CPUPercent: quietCPU}, KindIdleDatabase},
		{"linked database is in use", Resource{Ref: db("main"), LinkedApps: 1, Memory: mem, CPUPercent: quietCPU}, ""},
		{"unknown links never flag", Resource{Ref: db("main"), LinkedApps: -1, Memory: mem, CPUPercent: quietCPU}, ""},
		{"busy unlinked database", Resource{Ref: db("ext"), Memory: mem, CPUPercent: series(8*day, 5*time.Minute, 0.2, flat(15))}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := idleFinding(tc.res, th, testNow)
			if tc.want == "" {
				if ok {
					t.Fatalf("unexpected %s", f.Kind)
				}
				return
			}
			if !ok || f.Kind != tc.want {
				t.Fatalf("got %v %s, want %s", ok, f.Kind, tc.want)
			}
			if f.Impact.SavingMemoryBytes <= 0 {
				t.Fatal("idle finding should estimate a saving")
			}
			if tc.want == KindIdleApp && (f.Action.Type != ActionEnableSleep || f.Action.Value != th.SleepIdleMinutes) {
				t.Fatalf("action = %+v", f.Action)
			}
		})
	}
}

func TestTuningFinding(t *testing.T) {
	th := DefaultThresholds()
	tests := []struct {
		name     string
		res      Resource
		settings []string
		vector   bool
	}{
		{"pgvector postgres gets tuning and index note", Resource{Ref: db("askverdict"), Engine: "postgres", Version: "17-pgvector", Memory: series(2*day, 5*time.Minute, 0.05, flat(217*mib))}, []string{"shared_buffers", "work_mem", "max_connections", "effective_cache_size", "maintenance_work_mem"}, true},
		{"large plain postgres", Resource{Ref: db("big"), Engine: "postgres", Version: "16", MemoryLimit: 2 * gib, Memory: series(2*day, 5*time.Minute, 0.05, flat(900*mib))}, []string{"shared_buffers"}, false},
		{"small plain postgres is skipped", Resource{Ref: db("glinr"), Engine: "postgres", Version: "16", Memory: series(2*day, 5*time.Minute, 0.05, flat(126*mib))}, nil, false},
		{"redis gets maxmemory", Resource{Ref: db("cache"), Engine: "redis", Memory: series(2*day, 5*time.Minute, 0.05, flat(300*mib))}, []string{"maxmemory", "maxmemory-policy"}, false},
		{"mariadb gets buffer pool", Resource{Ref: db("shop"), Engine: "mariadb", Memory: series(2*day, 5*time.Minute, 0.05, flat(400*mib))}, []string{"innodb_buffer_pool_size"}, false},
		{"mongodb has no rule", Resource{Ref: db("docs"), Engine: "mongodb", Memory: series(2*day, 5*time.Minute, 0.05, flat(400*mib))}, nil, false},
		{"not enough data", Resource{Ref: db("new"), Engine: "postgres", Version: "17-pgvector", Memory: series(6*time.Hour, 5*time.Minute, 0, flat(217*mib))}, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := tuningFinding(tc.res, th, testNow)
			if ok != (tc.settings != nil) {
				t.Fatalf("found = %v", ok)
			}
			if !ok {
				return
			}
			for _, k := range tc.settings {
				if f.Action.Settings[k] == "" {
					t.Fatalf("missing setting %s in %v", k, f.Action.Settings)
				}
			}
			if f.Action.Appliable() {
				t.Fatal("tuning must never be appliable by the advisor")
			}
			if strings.Contains(f.Why, "pgvector") != tc.vector {
				t.Fatalf("pgvector note = %v, want %v", !tc.vector, tc.vector)
			}
		})
	}
}

func TestPostgresTuningValues(t *testing.T) {
	r := Resource{Ref: db("big"), Engine: "postgres", MemoryLimit: 2 * gib, Memory: series(2*day, 5*time.Minute, 0, flat(900*mib))}
	f, _ := tuningFinding(r, DefaultThresholds(), testNow)
	want := map[string]string{"shared_buffers": "512MB", "effective_cache_size": "1536MB", "max_connections": "100", "maintenance_work_mem": "128MB", "work_mem": "5MB"}
	for k, v := range want {
		if f.Action.Settings[k] != v {
			t.Errorf("%s = %s, want %s", k, f.Action.Settings[k], v)
		}
	}
}

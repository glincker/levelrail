package footprint

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestEmbeddedBaselineMatchesScriptsCopy(t *testing.T) {
	want, err := os.ReadFile("../../scripts/idle-footprint-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(idleBaselineJSON) {
		t.Fatal("internal/footprint/idle_baseline.json drifted from scripts/idle-footprint-baseline.json, copy it over")
	}
	if len(Baseline()) == 0 {
		t.Fatal("baseline did not parse")
	}
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestToleranceFromEnv(t *testing.T) {
	full := map[string]string{EnvTolRSSPct: "20", EnvTolGoroutines: "10", EnvTolFDs: "5", EnvTolCPUPoints: "1.5"}
	tol, err := ToleranceFromEnv(envOf(full))
	if err != nil || tol.RSSPercent != 20 || tol.Goroutines != 10 || tol.FDs != 5 || tol.CPUPoints != 1.5 {
		t.Fatalf("tol=%+v err=%v", tol, err)
	}
	if _, err := ToleranceFromEnv(envOf(nil)); err == nil {
		t.Fatal("unset tolerances must be an error")
	}
	full[EnvTolFDs] = "-1"
	if _, err := ToleranceFromEnv(envOf(full)); err == nil {
		t.Fatal("negative tolerance must be an error")
	}
}

func TestCPUWindowFromEnv(t *testing.T) {
	if got := CPUWindowFromEnv(envOf(nil)); got != 60*time.Second {
		t.Fatalf("default = %v", got)
	}
	if got := CPUWindowFromEnv(envOf(map[string]string{EnvCPUWindowSecs: "15"})); got != 15*time.Second {
		t.Fatalf("override = %v", got)
	}
}

func TestSelectBaselineNearestApps(t *testing.T) {
	rows := []BaselineRow{
		{OS: "linux", Arch: "arm64", Apps: 0, AppMode: "stopped"},
		{OS: "linux", Arch: "arm64", Apps: 20, AppMode: "stopped"},
		{OS: "linux", Arch: "arm64", Apps: 5, AppMode: "running"},
	}
	got, ok := SelectBaseline(rows, "linux", "arm64", 14)
	if !ok || got.Apps != 20 {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	if _, ok := SelectBaseline(rows, "darwin", "arm64", 0); ok {
		t.Fatal("other platform must not match")
	}
}

func TestAssess(t *testing.T) {
	rows := []BaselineRow{{OS: "linux", Arch: "arm64", AppMode: "stopped", PeakRSSMB: 200, CPUPercent: 0.4, Goroutines: 100, OpenFDs: 25}}
	tol := Tolerance{RSSPercent: 10, Goroutines: 10, FDs: 5, CPUPoints: 1}
	cur := Reading{RSSBytes: 210 << 20, CPUPercent: 0.9, Goroutines: 105, OpenFDs: 27}

	a := Assess(cur, rows, "linux", "arm64", tol, nil)
	if a.Status != StatusWithin || len(a.Checks) != 4 {
		t.Fatalf("within: %+v", a)
	}
	cur.Goroutines = 500
	a = Assess(cur, rows, "linux", "arm64", tol, nil)
	if a.Status != StatusOver {
		t.Fatalf("over: %+v", a)
	}
	var bad []string
	for _, c := range a.Checks {
		if !c.OK {
			bad = append(bad, c.Name)
		}
	}
	if !reflect.DeepEqual(bad, []string{"goroutines"}) {
		t.Fatalf("failing checks = %v", bad)
	}
	if a := Assess(cur, rows, "linux", "arm64", Tolerance{}, errors.New("unset")); a.Status != StatusToleranceOff || a.Baseline == nil {
		t.Fatalf("tolerance off: %+v", a)
	}
	if a := Assess(cur, rows, "darwin", "arm64", tol, nil); a.Status != StatusNoBaseline {
		t.Fatalf("no baseline: %+v", a)
	}
}

func TestWindowCPUUsesOldestSampleInsideWindow(t *testing.T) {
	base := time.Now()
	winMu.Lock()
	winSamples = []cpuSample{{at: base, cpu: 0}}
	winMu.Unlock()
	WindowCPU(time.Minute, base.Add(30*time.Second), 3)
	pct, covered := WindowCPU(time.Minute, base.Add(90*time.Second), 9)
	if covered < 59 || covered > 61 {
		t.Fatalf("covered = %v", covered)
	}
	if pct < 9.5 || pct > 10.5 {
		t.Fatalf("pct = %v, want about 10", pct)
	}
}

package benchreport

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

func reportWith(rps, warm float64, status string, repeat int) Report {
	conc := chaos.Result{Scenario: chaos.IngressConcurrency, Status: chaos.StatusPassed}
	conc.Set("achieved_rps", rps)
	dep := chaos.Result{Scenario: chaos.DeployLatency, Status: status}
	dep.Set("deploy_warm_p50_ms", warm)
	return New([]chaos.Result{dep, conc}, Options{
		Build: Build{Version: "v1", Commit: "abc"}, Host: mac(), Repeat: repeat, Now: time.Unix(1700000000, 0),
	})
}

func findDelta(t *testing.T, c Comparison, scenario, metric string) Delta {
	t.Helper()
	for _, d := range c.Deltas {
		if d.Scenario == scenario && d.Metric == metric {
			return d
		}
	}
	t.Fatalf("no delta for %s %s", scenario, metric)
	return Delta{}
}

func TestCompareDirections(t *testing.T) {
	base := reportWith(1000, 4000, chaos.StatusPassed, 3)
	head := reportWith(1500, 3000, chaos.StatusFailed, 3)
	c := Compare(base, head, Side{ID: "a"}, Side{ID: "b"}, 5)

	rps := findDelta(t, c, chaos.IngressConcurrency, "achieved_rps")
	if rps.Direction != DirectionBetter || rps.DeltaPct == nil || *rps.DeltaPct != 50 {
		t.Fatalf("rps = %+v", rps)
	}
	warm := findDelta(t, c, chaos.DeployLatency, "deploy_warm_p50_ms")
	if warm.Direction != DirectionBetter || *warm.Delta != -1000 {
		t.Fatalf("warm = %+v", warm)
	}
	if len(c.Status) != 1 || c.Status[0].Scenario != chaos.DeployLatency || c.Status[0].Head != chaos.StatusFailed {
		t.Fatalf("status changes = %+v", c.Status)
	}
	if c.Base.Runs != 3 || c.Statistic == "" {
		t.Fatalf("side = %+v", c.Base)
	}
}

func TestCompareNoiseAndMissingMetric(t *testing.T) {
	base := reportWith(1000, 4000, chaos.StatusPassed, 1)
	head := reportWith(1020, 4000, chaos.StatusPassed, 1)
	head.Results[1].Metrics["only_head"] = 7
	c := Compare(base, head, Side{}, Side{}, 5)
	if d := findDelta(t, c, chaos.IngressConcurrency, "achieved_rps"); d.Direction != DirectionSame {
		t.Fatalf("2%% change inside noise = %+v", d)
	}
	d := findDelta(t, c, chaos.IngressConcurrency, "only_head")
	if d.Base != nil || d.Head == nil || d.Delta != nil || d.Direction != "" {
		t.Fatalf("one sided metric = %+v", d)
	}
}

func TestCompareWarnings(t *testing.T) {
	base := reportWith(1, 1, chaos.StatusPassed, 1)
	head := reportWith(1, 1, chaos.StatusPassed, 1)
	head.Host.OS, head.Host.Arch, head.Build.Dirty = "linux", "amd64", true
	got := strings.Join(Compare(base, head, Side{}, Side{}, 5).Warnings, "\n")
	for _, want := range []string{"different platforms", "uncommitted", "single run", "NOT Linux"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings lack %q:\n%s", want, got)
		}
	}
}

func TestMedianOfRuns(t *testing.T) {
	mk := func(v float64) chaos.Result {
		r := chaos.Result{Scenario: chaos.DeployLatency, Status: chaos.StatusPassed}
		r.Set("deploy_cold_ms", v)
		return r
	}
	m := medians([]chaos.Result{mk(10), mk(100), mk(30)})
	if got := m[[2]string{chaos.DeployLatency, "deploy_cold_ms"}]; got != 30 {
		t.Fatalf("median = %v", got)
	}
	m = medians([]chaos.Result{mk(10), mk(20)})
	if got := m[[2]string{chaos.DeployLatency, "deploy_cold_ms"}]; got != 15 {
		t.Fatalf("even median = %v", got)
	}
}

func TestParseRoundTripAndRejects(t *testing.T) {
	rep := reportWith(1000, 4000, chaos.StatusPassed, 1)
	b, _ := json.Marshal(rep)
	got, err := Parse(b)
	if err != nil || got.Build.Commit != "abc" || len(got.Results) != 2 {
		t.Fatalf("parse = %+v err=%v", got, err)
	}
	if p, f, s := got.Tally(); p != 2 || f != 0 || s != 0 {
		t.Fatalf("tally %d %d %d", p, f, s)
	}

	bad := map[string]func(*Report){
		"schema":   func(r *Report) { r.SchemaVersion = 99 },
		"scenario": func(r *Report) { r.Results[0].Scenario = "nope" },
		"status":   func(r *Report) { r.Results[0].Status = "great" },
		"commit":   func(r *Report) { r.Build.Commit = "" },
	}
	for name, mutate := range bad {
		r := reportWith(1, 1, chaos.StatusPassed, 1)
		mutate(&r)
		raw, _ := json.Marshal(r)
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := Parse([]byte(`{"schema_version":1,"surprise":true}`)); err == nil {
		t.Error("unknown field accepted")
	}
}

func TestGroup(t *testing.T) {
	cases := map[string]string{
		chaos.ReplicaFailover: GroupFailover, chaos.KillContainer: GroupFailover, chaos.DeployLatency: GroupDeploy,
		chaos.RolloutUnderLoad: GroupRollout, chaos.IngressConcurrency: GroupThroughput, chaos.IngressSteady: GroupThroughput,
	}
	for s, want := range cases {
		if got := Group(s); got != want {
			t.Errorf("Group(%s) = %s, want %s", s, got, want)
		}
	}
}

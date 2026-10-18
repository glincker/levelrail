package migrationplan

import (
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		item platformimport.Item
		want Verdict
	}{
		{"mapped", platformimport.Item{Status: platformimport.StatusMapped}, WillImport},
		{"renamed only", platformimport.Item{Status: platformimport.StatusNeedsAttention, Reasons: []string{"name taken"}}, ImportsWithChanges},
		{"needs a manual step", platformimport.Item{Status: platformimport.StatusNeedsAttention, Reasons: []string{"x"}, Manual: []string{"do y"}}, NeedsManual},
		{"unsupported", platformimport.Item{Status: platformimport.StatusUnsupported}, Unsupported},
		{"already", platformimport.Item{Status: platformimport.StatusAlready}, AlreadyImported},
		{"created", platformimport.Item{Status: platformimport.StatusCreated}, AlreadyImported},
		{"skipped", platformimport.Item{Status: platformimport.StatusSkipped}, NeedsManual},
		{"failed", platformimport.Item{Status: platformimport.StatusFailed}, NeedsManual},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.item); got != tc.want {
				t.Fatalf("Classify = %s, want %s", got, tc.want)
			}
		})
	}
}

func fixture() *platformimport.Discovery {
	return &platformimport.Discovery{
		Platform: platformimport.Coolify,
		Apps: []platformimport.App{
			{SourceID: "a1", Name: "Web", Kind: platformimport.SourceImage, Image: "nginx:1", Port: 80,
				Env: []platformimport.Env{{Key: "A", Value: "1"}, {Key: "B", Value: "2"}}, Domains: []string{"web.example.com"}},
			{SourceID: "a2", Name: "api", Kind: platformimport.SourceImage, Image: "api:1",
				Volumes: []platformimport.Volume{{Name: "data", ContainerPath: "/data", SizeBytes: 10}},
				Crons:   []platformimport.Cron{{Name: "nightly", Schedule: "0 3 * * *", Command: "run"}}},
			{SourceID: "a3", Name: "site", Kind: platformimport.SourceImage, Image: "site:1", Port: 3000, Domains: []string{"taken.example.com"}},
		},
		Databases: []platformimport.Database{{SourceID: "d1", Name: "main", Engine: "postgres", Version: "16"}},
		Unsupported: []platformimport.Unsupported{
			{Kind: "service", SourceID: "s1", Name: "plausible", Source: "service plausible", Reason: "compose stack", Manual: "deploy compose"},
		},
	}
}

func TestAnalyzeVerdictsAndFixes(t *testing.T) {
	_, rep := Analyze(fixture(), Options{TakenDomains: map[string]string{"taken.example.com": "other"}})
	byID := map[string]Item{}
	for _, it := range rep.Items {
		byID[it.SourceID] = it
	}
	cases := []struct {
		id      string
		verdict Verdict
		fix     string
	}{
		{"a1", WillImport, FixCheckDNS},
		{"a2", NeedsManual, FixSetPort},
		{"a3", ImportsWithChanges, FixHoldDomains},
		{"d1", NeedsManual, FixCopyData},
		{"s1", Unsupported, FixUseTemplate},
	}
	for _, c := range cases {
		it, ok := byID[c.id]
		if !ok {
			t.Fatalf("%s missing from report", c.id)
		}
		if it.Verdict != c.verdict {
			t.Errorf("%s verdict = %s, want %s (reasons %v)", c.id, it.Verdict, c.verdict, it.Reasons)
		}
		found := false
		for _, f := range it.Fixes {
			found = found || f.Code == c.fix
		}
		if !found {
			t.Errorf("%s fixes = %v, want %s", c.id, it.Fixes, c.fix)
		}
	}
	if byID["a1"].EnvCount != 2 || byID["a2"].Volumes[0].SizeBytes != 10 || len(byID["a2"].Crons) != 1 {
		t.Errorf("inventory counts wrong: %+v", byID)
	}
	if rep.Counts[NeedsManual] != 2 || rep.Counts[Unsupported] != 1 {
		t.Errorf("counts = %v", rep.Counts)
	}
}

func TestAnalyzeOverridesClearBlockers(t *testing.T) {
	plan, rep := Analyze(fixture(), Options{
		TakenDomains: map[string]string{"taken.example.com": "other"},
		Overrides: map[string]Override{
			"a2": {Port: 8080, Name: "api-new"},
			"a3": {DropDomains: true},
		},
	})
	byID := map[string]Item{}
	for _, it := range rep.Items {
		byID[it.SourceID] = it
	}
	if got := byID["a3"]; len(got.Preview.Domains) != 0 {
		t.Errorf("domains not held back: %+v", got.Preview)
	}
	for _, f := range byID["a3"].Fixes {
		if f.Code == FixHoldDomains {
			t.Error("hold_domains still offered after it was applied")
		}
	}
	a2 := byID["a2"]
	if a2.Target != "api-new" || a2.Preview.Port != 8080 {
		t.Errorf("a2 override not applied: %+v", a2)
	}
	for _, f := range a2.Fixes {
		if f.Code == FixSetPort {
			t.Error("set_port still offered after a port was chosen")
		}
	}
	for _, p := range plan.Apps {
		if p.SourceID == "a2" && (p.Name != "api-new" || p.Port != 8080) {
			t.Errorf("plan not updated: %+v", p)
		}
	}
}

func TestAnalyzeRenameCollisionIgnored(t *testing.T) {
	_, rep := Analyze(fixture(), Options{Overrides: map[string]Override{"a2": {Name: "web"}}})
	if len(rep.Notes) == 0 {
		t.Fatal("expected a note about the ignored rename")
	}
}

func TestChecklistStateMachine(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	c := New()
	if c.Current() != StepImported {
		t.Fatalf("current = %q", c.Current())
	}
	tests := []struct {
		name string
		do   func() error
		want error
	}{
		{"skip ahead is blocked", func() error { return c.Complete(StepDNSSwitched, "u", "", nil, now) }, ErrStepBlocked},
		{"unknown step", func() error { return c.Complete("nope", "u", "", nil, now) }, ErrUnknownStep},
		{"imported", func() error { return c.Complete(StepImported, "u", "", nil, now) }, nil},
		{"imported twice", func() error { return c.Complete(StepImported, "u", "", nil, now) }, ErrAlreadyDone},
		{"data after imported", func() error { return c.Complete(StepDataVerified, "u", "", []string{"ok"}, now) }, nil},
		{"old stopped before dns", func() error { return c.Complete(StepOldStopped, "u", "", nil, now) }, ErrStepBlocked},
		{"reset a pending step", func() error { return c.Reset(StepShadowRunning) }, ErrStepNotDone},
		{"shadow", func() error { return c.Complete(StepShadowRunning, "u", "", nil, now) }, nil},
		{"dns", func() error { return c.Complete(StepDNSSwitched, "u", "", nil, now) }, nil},
		{"old stopped", func() error { return c.Complete(StepOldStopped, "u", "", nil, now) }, nil},
	}
	for _, tc := range tests {
		if err := tc.do(); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	if c.Current() != "" || c.Progress() != 5 {
		t.Fatalf("finished checklist: current %q progress %d", c.Current(), c.Progress())
	}
	if err := c.Reset(StepDataVerified); err != nil {
		t.Fatal(err)
	}
	if c.Current() != StepDataVerified || c.Progress() != 1 {
		t.Fatalf("after reset: current %q progress %d", c.Current(), c.Progress())
	}
	back, err := Parse(c.Marshal())
	if err != nil || back.Progress() != 1 || back.States[StepImported].Actor != "u" {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	if _, err := Parse("{not json"); !errors.Is(err, ErrBadChecklist) {
		t.Fatalf("bad json err = %v", err)
	}
}

func TestVerifyData(t *testing.T) {
	tests := []struct {
		name     string
		dbs      []DatabaseResult
		want     []string
		stats    []VolumeStat
		problems int
	}{
		{"nothing to compare", nil, nil, nil, 0},
		{"db verified", []DatabaseResult{{Name: "m", Status: "verified", Checked: 3, Found: true}}, nil, nil, 0},
		{"db mismatch", []DatabaseResult{{Name: "m", Status: "verified", Checked: 3, Mismatched: 1, Found: true}}, nil, nil, 1},
		{"db never copied", []DatabaseResult{{Name: "m"}}, nil, nil, 1},
		{"db failed", []DatabaseResult{{Name: "m", Status: "failed", Found: true}}, nil, nil, 1},
		{"volume missing stat", nil, []string{"v"}, nil, 1},
		{"volume equal", nil, []string{"v"}, []VolumeStat{{Name: "v", SourceFiles: 4, SourceBytes: 9, TargetFiles: 4, TargetBytes: 9}}, 0},
		{"volume size differs", nil, []string{"v"}, []VolumeStat{{Name: "v", SourceFiles: 4, SourceBytes: 9, TargetFiles: 4, TargetBytes: 8}}, 1},
		{"volume count differs", nil, []string{"v"}, []VolumeStat{{Name: "v", SourceFiles: 4, SourceBytes: 9, TargetFiles: 3, TargetBytes: 9}}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, problems := VerifyData(tc.dbs, tc.want, tc.stats)
			if len(problems) != tc.problems {
				t.Fatalf("problems = %v, want %d", problems, tc.problems)
			}
			if tc.problems == 0 && len(ev) == 0 {
				t.Fatal("a passing check must leave evidence")
			}
		})
	}
}

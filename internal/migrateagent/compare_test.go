package migrateagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/migrationplan"
)

func TestSourceTarget(t *testing.T) {
	cases := []struct{ app, vol, want string }{
		{"blog", "app-blog-uploads", "uploads"},
		{"blog", "app-blog-app-blog-x", "app-blog-x"},
		{"blog", "other-vol", "other-vol"},
		{"blog", "/srv/data", "/srv/data"},
	}
	for _, tc := range cases {
		if got := SourceTarget(tc.app, tc.vol); got != tc.want {
			t.Errorf("SourceTarget(%q,%q)=%q want %q", tc.app, tc.vol, got, tc.want)
		}
	}
}

func TestCompare(t *testing.T) {
	vols := []AppVolume{{Name: "app-a-data", Source: "data"}}
	cases := []struct {
		name   string
		scan   []StatDoc
		target map[string]TargetStat
		want   string
	}{
		{"match", []StatDoc{{Target: "data", Files: 3, Bytes: 30}}, map[string]TargetStat{"app-a-data": {Files: 3, Bytes: 30}}, StatusMatch},
		{"file count differs", []StatDoc{{Target: "data", Files: 3, Bytes: 30}}, map[string]TargetStat{"app-a-data": {Files: 2, Bytes: 30}}, StatusDiffers},
		{"bytes differ", []StatDoc{{Target: "data", Files: 3, Bytes: 30}}, map[string]TargetStat{"app-a-data": {Files: 3, Bytes: 31}}, StatusDiffers},
		{"changed during scan blocks an otherwise equal pair", []StatDoc{{Target: "data", Files: 3, Bytes: 30, ChangedDuringScan: true}}, map[string]TargetStat{"app-a-data": {Files: 3, Bytes: 30}}, StatusChanged},
		{"source missing", nil, map[string]TargetStat{"app-a-data": {Files: 3, Bytes: 30}}, StatusSourceMissing},
		{"scan error", []StatDoc{{Target: "data", Error: "boom"}}, map[string]TargetStat{"app-a-data": {Files: 3, Bytes: 30}}, StatusScanError},
		{"target missing", []StatDoc{{Target: "data", Files: 3, Bytes: 30}}, nil, StatusTargetMissing},
		{"target error", []StatDoc{{Target: "data", Files: 3, Bytes: 30}}, map[string]TargetStat{"app-a-data": {Err: "no such dir"}}, StatusTargetMissing},
		{"both empty is a match", []StatDoc{{Target: "data"}}, map[string]TargetStat{"app-a-data": {}}, StatusMatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Compare(vols, tc.scan, tc.target)
			if len(got) != 1 || got[0].Status != tc.want {
				t.Fatalf("got %+v want status %s", got, tc.want)
			}
		})
	}
}

func checklistAt(t *testing.T, done ...string) migrationplan.Checklist {
	t.Helper()
	c := migrationplan.New()
	for _, s := range done {
		if err := c.Complete(s, "op", "", nil, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func matching(n int) []Comparison {
	out := make([]Comparison, n)
	for i := range out {
		out[i] = Comparison{Name: "v", Status: StatusMatch, SourceFiles: 2, SourceBytes: 9, TargetFiles: 2, TargetBytes: 9}
	}
	return out
}

func TestAutoAdvance(t *testing.T) {
	cases := []struct {
		name     string
		done     []string
		cs       []Comparison
		dbs      int
		advanced bool
	}{
		{"zero diffs advances", []string{migrationplan.StepImported}, matching(2), 0, true},
		{"advances and notes databases", []string{migrationplan.StepImported}, matching(1), 2, true},
		{"one diff blocks", []string{migrationplan.StepImported}, append(matching(1), Comparison{Name: "w", Status: StatusDiffers}), 0, false},
		{"changed during scan blocks", []string{migrationplan.StepImported}, []Comparison{{Name: "v", Status: StatusChanged}}, 0, false},
		{"no volumes does not advance", []string{migrationplan.StepImported}, nil, 0, false},
		{"imported not done", nil, matching(1), 0, false},
		{"already past data_verified", []string{migrationplan.StepImported, migrationplan.StepDataVerified}, matching(1), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := checklistAt(t, tc.done...)
			res := AutoAdvance(&c, tc.cs, tc.dbs, AdvanceOptions{}, time.Now())
			if res.Advanced != tc.advanced {
				t.Fatalf("advanced=%v reason=%q want %v", res.Advanced, res.Reason, tc.advanced)
			}
			st := c.States[migrationplan.StepDataVerified]
			if tc.advanced && (st.Status != migrationplan.StatusDone || st.Actor != Actor || len(st.Evidence) == 0) {
				t.Fatalf("step not recorded as the agent's: %+v", st)
			}
			if !tc.advanced && len(tc.done) < 2 && st.Status == migrationplan.StatusDone {
				t.Fatalf("step advanced when it must not: %+v", st)
			}
		})
	}
}

func TestAutoAdvanceNeverTouchesLaterSteps(t *testing.T) {
	c := checklistAt(t, migrationplan.StepImported)
	AutoAdvance(&c, matching(1), 0, AdvanceOptions{}, time.Now())
	for _, s := range []string{migrationplan.StepShadowRunning, migrationplan.StepDNSSwitched, migrationplan.StepOldStopped} {
		if c.States[s].Status == migrationplan.StatusDone {
			t.Fatalf("%s was completed by the agent", s)
		}
	}
}

func TestDatabaseNoteInEvidence(t *testing.T) {
	c := checklistAt(t, migrationplan.StepImported)
	res := AutoAdvance(&c, matching(1), 1, AdvanceOptions{}, time.Now())
	last := res.Evidence[len(res.Evidence)-1]
	if !res.Advanced || last == "" || last[0] != '1' {
		t.Fatalf("database caveat missing from evidence: %v", res.Evidence)
	}
}

func TestReopenUndoesAgentAdvanceOnly(t *testing.T) {
	c := checklistAt(t, migrationplan.StepImported)
	AutoAdvance(&c, matching(1), 0, AdvanceOptions{}, time.Now())
	if err := c.Complete(migrationplan.StepShadowRunning, "op", "", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Reopen(&c); err != nil {
		t.Fatal(err)
	}
	if c.States[migrationplan.StepDataVerified].Status != migrationplan.StatusPending || c.States[migrationplan.StepShadowRunning].Status != migrationplan.StatusPending {
		t.Fatalf("reopen did not reset data_verified and what depended on it: %+v", c.States)
	}
	if c.Current() != migrationplan.StepDataVerified {
		t.Fatalf("current step %q after reopen", c.Current())
	}
	manual := checklistAt(t, migrationplan.StepImported, migrationplan.StepDataVerified)
	if err := Reopen(&manual); !errors.Is(err, ErrNotAgentStep) {
		t.Fatalf("reopen of a human-completed step got %v, want ErrNotAgentStep", err)
	}
}

func TestMeasureTarget(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "vol", "_data")
	if err := os.MkdirAll(data, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "f"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := MeasureTarget(context.Background(), dir, "vol"); got.Files != 1 || got.Bytes != 5 || got.Err != "" {
		t.Fatalf("volume: %+v", got)
	}
	if got := MeasureTarget(context.Background(), dir, "missing"); got.Err == "" {
		t.Fatal("missing volume must report an error")
	}
	if got := MeasureTarget(context.Background(), dir, "../vol"); got.Err == "" {
		t.Fatal("traversal must be refused")
	}
	if got := MeasureTarget(context.Background(), dir, data); got.Files != 1 {
		t.Fatalf("bind path: %+v", got)
	}
}

func TestMarkUnlisted(t *testing.T) {
	inv := InventoryDoc{Containers: []ContainerDoc{{Name: "blog-web-1"}, {Name: "abc123-db"}, {Name: "stray"}}}
	MarkUnlisted(&inv, []ImportedRef{{App: "blog"}, {SourceID: "ABC123"}})
	got := []bool{inv.Containers[0].Unlisted, inv.Containers[1].Unlisted, inv.Containers[2].Unlisted}
	want := []bool{false, false, true}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("container %d unlisted=%v want %v", i, got[i], want[i])
		}
	}
}

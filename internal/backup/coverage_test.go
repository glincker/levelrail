package backup

import "testing"

func TestDeriveCoverage(t *testing.T) {
	backup := &HealthBackup{ID: "b1", At: "2026-10-10T03:00:00Z", Status: "succeeded"}
	drill := &DrillSummary{ID: "d1", At: "2026-10-09T03:00:00Z", Status: "passed"}
	scheduled := CoverageResource{Kind: "database", Name: "a", Schedule: "0 3 * * *", TargetID: "t"}
	cases := []struct {
		name    string
		res     CoverageResource
		health  *ResourceHealth
		want    string
		action  string
		counted bool
	}{
		{"no schedule no history", CoverageResource{Kind: "database", Name: "a"}, nil, CoverageUnprotected, ActionProtect, true},
		{"manual backup only", CoverageResource{Kind: "database", Name: "a"}, &ResourceHealth{Kind: "database", ResourceName: "a", LastBackup: backup}, CoverageUnprotected, ActionProtect, true},
		{"schedule without target", CoverageResource{Kind: "database", Name: "a", Schedule: "0 3 * * *"}, nil, CoverageUnprotected, ActionProtect, true},
		{"scheduled never ran", scheduled, nil, CoverageNeverRan, ActionBackupNow, true},
		{"scheduled all attempts failed", scheduled,
			&ResourceHealth{Kind: "database", ResourceName: "a", State: HealthFailing, StateReason: "every backup attempt has failed"}, CoverageFailing, ActionBackupNow, true},
		{"last drill failed", scheduled,
			&ResourceHealth{Kind: "database", ResourceName: "a", LastBackup: backup, State: HealthFailing}, CoverageFailing, ActionBackupNow, true},
		{"backup but unverified", scheduled,
			&ResourceHealth{Kind: "database", ResourceName: "a", LastBackup: backup, State: HealthUnverified}, CoverageUnverified, ActionRestore, true},
		{"verified", scheduled,
			&ResourceHealth{Kind: "database", ResourceName: "a", LastBackup: backup, LastVerifiedRestore: drill, State: HealthHealthy}, CoverageVerified, ActionRestore, true},
		{"scratch left out of totals", CoverageResource{Kind: "database", Name: "a", Scratch: true}, nil, CoverageUnprotected, ActionProtect, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var health []ResourceHealth
			if tc.health != nil {
				health = append(health, *tc.health)
			}
			rows, sum := DeriveCoverage([]CoverageResource{tc.res}, health)
			if len(rows) != 1 || rows[0].State != tc.want || rows[0].PrimaryAction != tc.action {
				t.Fatalf("got %+v, want state %s action %s", rows, tc.want, tc.action)
			}
			if (sum.Total == 1) != tc.counted {
				t.Fatalf("total = %d, counted = %v", sum.Total, tc.counted)
			}
		})
	}
}

func TestDeriveCoverageSummary(t *testing.T) {
	b := &HealthBackup{ID: "b", At: "2026-10-10T03:00:00Z"}
	d := &DrillSummary{At: "2026-10-09T03:00:00Z"}
	res := []CoverageResource{
		{Kind: "database", Name: "pg", Schedule: "0 3 * * *", TargetID: "t"},
		{Kind: "volume", AppName: "web", Name: "uploads"},
		{Kind: "volume", AppName: "web", Name: "cache", Scratch: true},
	}
	health := []ResourceHealth{
		{Kind: "database", ResourceName: "pg", LastBackup: b, LastVerifiedRestore: d, TotalBytes: 100, NextRun: "2026-10-11T03:00:00Z"},
		{Kind: "database", ResourceName: "deleted", LastBackup: b, TotalBytes: 999},
	}
	rows, sum := DeriveCoverage(res, health)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if sum.Total != 2 || sum.Protected != 1 || sum.Unprotected != 1 || sum.Scratch != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if sum.StorageBytes != 100 || sum.LastVerifiedRestore != d.At || sum.NextRun != "2026-10-11T03:00:00Z" {
		t.Fatalf("summary = %+v", sum)
	}
	if rows[0].Name != "uploads" || !rows[2].Scratch {
		t.Fatalf("order = %+v", rows)
	}
}

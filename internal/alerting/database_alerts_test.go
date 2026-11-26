package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/kit/forecast"
)

type fakeDBAlertSource struct{ in []DatabaseAlertInput }

func (f fakeDBAlertSource) DatabaseAlertInputs(context.Context) ([]DatabaseAlertInput, error) {
	return f.in, nil
}

func testDBPolicy() DatabaseAlertPolicy {
	return DatabaseAlertPolicy{
		CrashLoopEnabled: true, DiskGrowthEnabled: true, DiskWarnDays: 14, DiskCriticalDays: 3,
		ConnectionsEnabled: true, ConnectionsPercent: 90, BackupEnabled: true,
		BackupWindow: 26 * time.Hour, BackupUnprotectedBytes: 512 << 20,
	}
}

// growingDisk fills fillPerDay bytes a day over 3 days with the volume 100 units wide.
func growingDisk(now time.Time, usedNow, perDay, total float64) ([]forecast.Point, float64) {
	var pts []forecast.Point
	for h := 72; h >= 0; h -= 6 {
		pts = append(pts, forecast.Point{Timestamp: now.Add(-time.Duration(h) * time.Hour), UsedBytes: usedNow - perDay*float64(h)/24})
	}
	return pts, total
}

func TestDatabaseBreachesTable(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	p := testDBPolicy()
	fast, total := growingDisk(now, 80, 10, 100)  // 2 days left
	slow, _ := growingDisk(now, 40, 1, 100)       // 60 days left
	nearWarn, _ := growingDisk(now, 70, 2.3, 100) // about 13 days left
	cases := []struct {
		name      string
		kind      Kind
		severity  string
		in        DatabaseAlertInput
		policy    func(*DatabaseAlertPolicy)
		wasFiring bool
		want      bool
		contains  string
	}{
		{"crash breach", KindDatabaseCrashLoop, SeverityCritical, DatabaseAlertInput{Name: "pg", CrashLooping: true}, nil, false, true, "/databases/pg/monitoring"},
		{"crash fine", KindDatabaseCrashLoop, SeverityCritical, DatabaseAlertInput{Name: "pg"}, nil, false, false, ""},
		{"crash disabled", KindDatabaseCrashLoop, SeverityCritical, DatabaseAlertInput{Name: "pg", CrashLooping: true}, func(p *DatabaseAlertPolicy) { p.CrashLoopEnabled = false }, false, false, ""},

		{"conn breach", KindDatabaseConnectionsSaturated, SeverityWarning, DatabaseAlertInput{Name: "pg", ConnTotal: 95, ConnMax: 100}, nil, false, true, "95%"},
		{"conn fine", KindDatabaseConnectionsSaturated, SeverityWarning, DatabaseAlertInput{Name: "pg", ConnTotal: 50, ConnMax: 100}, nil, false, false, ""},
		{"conn hysteresis holds", KindDatabaseConnectionsSaturated, SeverityWarning, DatabaseAlertInput{Name: "pg", ConnTotal: 87, ConnMax: 100}, nil, true, true, ""},
		{"conn hysteresis clears", KindDatabaseConnectionsSaturated, SeverityWarning, DatabaseAlertInput{Name: "pg", ConnTotal: 87, ConnMax: 100}, nil, false, false, ""},
		{"conn no max", KindDatabaseConnectionsSaturated, SeverityWarning, DatabaseAlertInput{Name: "pg", ConnTotal: 5}, nil, false, false, ""},

		{"disk slow growth", KindDatabaseDiskGrowth, SeverityWarning, DatabaseAlertInput{Name: "pg", DiskPoints: slow, DiskTotalBytes: total}, nil, false, false, ""},
		{"disk fast warn", KindDatabaseDiskGrowth, SeverityWarning, DatabaseAlertInput{Name: "pg", DiskPoints: fast, DiskTotalBytes: total}, nil, false, true, "/databases/pg/monitoring"},
		{"disk fast critical", KindDatabaseDiskGrowth, SeverityCritical, DatabaseAlertInput{Name: "pg", DiskPoints: fast, DiskTotalBytes: total}, nil, false, true, "days"},
		{"disk slow critical", KindDatabaseDiskGrowth, SeverityCritical, DatabaseAlertInput{Name: "pg", DiskPoints: nearWarn, DiskTotalBytes: total}, nil, false, false, ""},
		{"disk no history", KindDatabaseDiskGrowth, SeverityWarning, DatabaseAlertInput{Name: "pg", DiskTotalBytes: total}, nil, false, false, ""},
		{"disk hysteresis holds", KindDatabaseDiskGrowth, SeverityWarning, DatabaseAlertInput{Name: "pg", DiskPoints: nearWarn, DiskTotalBytes: total}, func(p *DatabaseAlertPolicy) { p.DiskWarnDays = 12 }, true, true, ""},
		{"disk hysteresis clears", KindDatabaseDiskGrowth, SeverityWarning, DatabaseAlertInput{Name: "pg", DiskPoints: nearWarn, DiskTotalBytes: total}, func(p *DatabaseAlertPolicy) { p.DiskWarnDays = 12 }, false, false, ""},

		{"backup stale breach", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, BackupSchedule: "0 3 * * *", LastBackupAt: now.Add(-40 * time.Hour)}, nil, false, true, "section=backups"},
		{"backup fresh", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, BackupSchedule: "0 3 * * *", LastBackupAt: now.Add(-3 * time.Hour)}, nil, false, false, ""},
		{"backup never since create", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, BackupSchedule: "0 3 * * *", CreatedAt: now.Add(-48 * time.Hour)}, nil, false, true, "no successful backup"},
		{"backup new database", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, BackupSchedule: "0 3 * * *", CreatedAt: now.Add(-2 * time.Hour)}, nil, false, false, ""},
		{"backup no schedule", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, LastBackupAt: now.Add(-400 * time.Hour)}, nil, false, false, ""},
		{"backup hysteresis holds", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, BackupSchedule: "0 3 * * *", LastBackupAt: now.Add(-25 * time.Hour)}, nil, true, true, ""},
		{"backup hysteresis clears", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", BackupTarget: true, BackupSchedule: "0 3 * * *", LastBackupAt: now.Add(-25 * time.Hour)}, nil, false, false, ""},
		{"no target big", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", SizeBytes: 2 << 30}, nil, false, true, "no backup storage"},
		{"no target small", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", SizeBytes: 10 << 20}, nil, false, false, ""},
		{"no target warning off", KindDatabaseBackupMissing, SeverityWarning, DatabaseAlertInput{Name: "pg", SizeBytes: 2 << 30}, func(p *DatabaseAlertPolicy) { p.BackupUnprotectedBytes = 0 }, false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pol := p
			if c.policy != nil {
				c.policy(&pol)
			}
			got := DatabaseBreaches(c.kind, c.severity, []DatabaseAlertInput{c.in}, pol, c.wasFiring, now)
			if (len(got) > 0) != c.want {
				t.Fatalf("breach = %v, want %v (%+v)", len(got) > 0, c.want, got)
			}
			if c.want && !strings.Contains(got[0].Notice, c.contains) {
				t.Fatalf("notice %q lacks %q", got[0].Notice, c.contains)
			}
			if c.want && !strings.Contains(got[0].Notice, "pg") {
				t.Fatalf("notice %q lacks database name", got[0].Notice)
			}
		})
	}
}

func TestEvaluateDatabaseAlertForDurationAndResolve(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	pol := testDBPolicy()
	pol.ConnectionsFor = 5 * time.Minute
	hot := fakeDBAlertSource{in: []DatabaseAlertInput{{Name: "pg", ConnTotal: 99, ConnMax: 100}}}
	cool := fakeDBAlertSource{in: []DatabaseAlertInput{{Name: "pg", ConnTotal: 10, ConnMax: 100}}}
	r := Rule{ID: "r", Kind: KindDatabaseConnectionsSaturated, Severity: SeverityWarning}

	r, notice, err := EvaluateDatabaseAlert(context.Background(), hot, pol, r, now)
	if err != nil || r.Firing || r.PendingSince == nil || notice == "" {
		t.Fatalf("first tick should be pending: %+v %q %v", r, notice, err)
	}
	r, _, _ = EvaluateDatabaseAlert(context.Background(), hot, pol, r, now.Add(6*time.Minute))
	if !r.Firing {
		t.Fatalf("should fire after the for-duration: %+v", r)
	}
	r, notice, _ = EvaluateDatabaseAlert(context.Background(), cool, pol, r, now.Add(7*time.Minute))
	if r.Firing || r.PendingSince != nil || notice != "" {
		t.Fatalf("should resolve: %+v %q", r, notice)
	}
}

func TestDatabaseAlertingNamesAggregates(t *testing.T) {
	now := time.Now()
	in := []DatabaseAlertInput{
		{Name: "b", CrashLooping: true, ConnTotal: 99, ConnMax: 100},
		{Name: "a", ConnTotal: 99, ConnMax: 100},
		{Name: "ok", ConnTotal: 1, ConnMax: 100, BackupTarget: true, LastBackupAt: now},
	}
	got := DatabaseAlertingNames(in, testDBPolicy(), now)
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("names = %v", got)
	}
}

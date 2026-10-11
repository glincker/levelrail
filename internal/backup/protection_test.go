package backup

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type protStore struct {
	*memHistory
	attempts []store.BackupHistory
	vols     []store.ServiceVolumeBackupConfig
	drills   []store.BackupDrill
	prot     []store.BackupTargetProtection
}

func (p *protStore) ListAllBackupHistory(context.Context, int, *time.Time) ([]store.BackupHistory, error) {
	return p.attempts, nil
}
func (p *protStore) ListScheduledServiceVolumes(context.Context) ([]store.ServiceVolumeBackupConfig, error) {
	return p.vols, nil
}
func (p *protStore) ListScheduledDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return nil, nil
}
func (p *protStore) ListBackupDrills(context.Context, string, string, string, int) ([]store.BackupDrill, error) {
	return p.drills, nil
}
func (p *protStore) ListBackupTargetProtection(context.Context) ([]store.BackupTargetProtection, error) {
	return p.prot, nil
}
func (p *protStore) SetVolumeBackupPolicy(context.Context, store.VolumeBackupPolicy) error {
	return nil
}

func TestProtection_HealthStates(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	ok := func(id, at string) store.BackupHistory {
		return store.BackupHistory{ID: id, ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", TargetID: "bkt_test", Status: store.BackupStatusSucceeded, StartedAt: at, SizeBytes: 100, Codec: CodecZstdAge}
	}
	failed := func(at string) store.BackupHistory {
		return store.BackupHistory{ID: "f", ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", TargetID: "bkt_test", Status: store.BackupStatusFailed, StartedAt: at, Error: "boom"}
	}
	drill := func(status, at string) store.BackupDrill {
		return store.BackupDrill{ID: "d" + at, ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", Status: status, StartedAt: at, Stage: "done"}
	}
	cases := []struct {
		name     string
		attempts []store.BackupHistory
		drills   []store.BackupDrill
		prot     []store.BackupTargetProtection
		want     string
	}{
		{"never succeeded", []store.BackupHistory{failed("2026-10-09T00:00:00Z")}, nil, nil, HealthFailing},
		{"no backups at all", nil, nil, nil, HealthUnprotected},
		{"backed up but never drilled", []store.BackupHistory{ok("a", "2026-10-09T00:00:00Z")}, nil, nil, HealthUnverified},
		{"verified and healthy", []store.BackupHistory{ok("a", "2026-10-09T00:00:00Z")}, []store.BackupDrill{drill("passed", "2026-10-09T01:00:00Z")}, []store.BackupTargetProtection{{TargetID: "bkt_test", ObjectLock: true}}, HealthHealthy},
		{"last drill failed", []store.BackupHistory{ok("a", "2026-10-09T00:00:00Z")}, []store.BackupDrill{drill("failed", "2026-10-09T02:00:00Z"), drill("passed", "2026-10-08T01:00:00Z")}, nil, HealthFailing},
		{"newest attempt failed after a good one", []store.BackupHistory{failed("2026-10-09T05:00:00Z"), ok("a", "2026-10-09T00:00:00Z")}, []store.BackupDrill{drill("passed", "2026-10-09T01:00:00Z")}, nil, HealthWarning},
		{"verified restore overdue", []store.BackupHistory{ok("a", "2026-10-09T00:00:00Z")}, []store.BackupDrill{drill("passed", "2026-09-01T01:00:00Z")}, nil, HealthWarning},
		{"open bucket warns", []store.BackupHistory{ok("a", "2026-10-09T00:00:00Z")}, []store.BackupDrill{drill("passed", "2026-10-09T01:00:00Z")}, []store.BackupTargetProtection{{TargetID: "bkt_test", CanDelete: true}}, HealthWarning},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &protStore{memHistory: newMemHistory(), attempts: c.attempts, drills: c.drills, prot: c.prot,
				vols: []store.ServiceVolumeBackupConfig{{ServiceName: "web", VolumeName: "data", BackupTargetID: "bkt_test", BackupSchedule: "0 3 * * *"}}}
			p := &Protection{Store: st, DrillInterval: 7 * 24 * time.Hour, Now: func() time.Time { return now }}
			rows, err := p.Health(context.Background())
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows = %d err = %v", len(rows), err)
			}
			r := rows[0]
			if r.State != c.want {
				t.Fatalf("state = %q (%s), want %q", r.State, r.StateReason, c.want)
			}
			if r.NextRun != "2026-10-10T03:00:00Z" {
				t.Fatalf("next run = %q", r.NextRun)
			}
			if c.name == "verified and healthy" && (r.LastVerifiedRestore == nil || r.TotalBytes != 100 || !r.Encrypted || r.ProtectionLevel != ProtectionLocked) {
				t.Fatalf("row = %+v", r)
			}
		})
	}
}

func TestProtection_FailedDrillsClearOnLaterPass(t *testing.T) {
	d := func(id, status, at string) store.BackupDrill {
		return store.BackupDrill{ID: id, ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", Status: status, StartedAt: at, Stage: "restore", Error: "boom"}
	}
	st := &protStore{memHistory: newMemHistory(), drills: []store.BackupDrill{d("2", "failed", "2026-10-02T00:00:00Z"), d("1", "passed", "2026-10-01T00:00:00Z")}}
	p := &Protection{Store: st}
	got, err := p.FailedDrills(context.Background())
	if err != nil || len(got) != 1 || got[0].Resource != "web/data" {
		t.Fatalf("failed = %+v err = %v", got, err)
	}
	st.drills = []store.BackupDrill{d("3", "passed", "2026-10-03T00:00:00Z"), d("2", "failed", "2026-10-02T00:00:00Z")}
	if got, _ = p.FailedDrills(context.Background()); len(got) != 0 {
		t.Fatalf("a later passing drill must clear the failure: %+v", got)
	}
}

func TestValidateVolumeName(t *testing.T) {
	for name, ok := range map[string]bool{"app-web-data": true, "clone_1.x": true, "": false, "-lead": false, "has space": false, "semi;colon": false} {
		if err := ValidateVolumeName(name); (err == nil) != ok {
			t.Fatalf("ValidateVolumeName(%q) = %v, want ok=%v", name, err, ok)
		}
	}
}

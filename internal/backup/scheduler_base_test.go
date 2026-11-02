package backup

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeBaseScheduleStore struct {
	*fakeScheduleStore
	baseDBs []store.DesiredDatabase
}

func (f *fakeBaseScheduleStore) ListBaseBackupScheduledDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return f.baseDBs, nil
}

func baseTick(t *testing.T, s *Scheduler, hour int) {
	t.Helper()
	s.Now = func() time.Time { return time.Date(2026, 8, 15, hour, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
}

func TestScheduler_Tick_BaseBackupRunsWithoutDumpSchedule(t *testing.T) {
	d := store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, PITREnabled: true, BaseBackupSchedule: "0 3 * * *", BaseBackupTargetID: "bkt_base", BackupTargetID: "bkt_dump"}
	runner := &fakeScheduledRunner{}
	base := &fakeScheduledBaseBackups{}
	s := NewScheduler(&fakeBaseScheduleStore{fakeScheduleStore: &fakeScheduleStore{}, baseDBs: []store.DesiredDatabase{d}}, runner, nil, nil)
	s.BaseBackups = base

	baseTick(t, s, 1)
	if len(base.calls) != 0 {
		t.Fatalf("armed tick ran a base backup: %v", base.calls)
	}
	baseTick(t, s, 3)
	if len(base.calls) != 1 || base.calls[0] != "main|db-main|bkt_base" {
		t.Fatalf("base calls = %v, want one for bkt_base", base.calls)
	}
	baseTick(t, s, 3)
	if len(base.calls) != 1 {
		t.Fatalf("same slot ran again: %v", base.calls)
	}
}

func TestScheduler_Tick_BaseBackupFallsBackToDumpTarget(t *testing.T) {
	d := store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, PITREnabled: true, BaseBackupSchedule: "0 3 * * *", BackupTargetID: "bkt_dump"}
	base := &fakeScheduledBaseBackups{}
	s := NewScheduler(&fakeBaseScheduleStore{fakeScheduleStore: &fakeScheduleStore{}, baseDBs: []store.DesiredDatabase{d}}, &fakeScheduledRunner{}, nil, nil)
	s.BaseBackups = base

	baseTick(t, s, 1)
	baseTick(t, s, 3)
	if len(base.calls) != 1 || base.calls[0] != "main|db-main|bkt_dump" {
		t.Fatalf("base calls = %v, want dump target fallback", base.calls)
	}
}

func TestScheduler_Tick_OwnBaseScheduleSkipsPostDumpBaseBackup(t *testing.T) {
	d := scheduledDB("main", "0 3 * * *", 0)
	d.PITREnabled = true
	d.BaseBackupSchedule = "0 4 * * 0"
	base := &fakeScheduledBaseBackups{}
	s := NewScheduler(&fakeScheduleStore{dbs: []store.DesiredDatabase{d}}, &fakeScheduledRunner{}, nil, nil)
	s.BaseBackups = base

	baseTick(t, s, 1)
	baseTick(t, s, 3)
	if len(base.calls) != 0 {
		t.Fatalf("dump run triggered a base backup despite its own schedule: %v", base.calls)
	}
}

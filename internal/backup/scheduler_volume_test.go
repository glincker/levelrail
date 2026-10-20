package backup

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func scheduledVolume(retain int) store.ServiceVolumeBackupConfig {
	return store.ServiceVolumeBackupConfig{
		ServiceName: "web", VolumeName: "data",
		BackupTargetID: "bkt_1", BackupSchedule: "0 3 * * *", BackupRetain: retain,
	}
}

// TestScheduler_Tick_Volume_FiresOnceDue_PassesSqlitePath covers the
// sqlite_path passthrough runScheduledVolume's RunVolumeBackup call added.
func TestScheduler_Tick_Volume_FiresOnceDue_PassesSqlitePath(t *testing.T) {
	v := scheduledVolume(0)
	v.SqlitePath = "app/data.db"
	fakeStore := &fakeScheduleStore{
		volumes:           []store.ServiceVolumeBackupConfig{v},
		resolveVolumeName: map[string]string{"web/data": "app-web-data"},
	}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(runner.volumeCalls) != 1 {
		t.Fatalf("volume backup calls = %d, want 1", len(runner.volumeCalls))
	}
	if got := runner.volumeCalls[0].sqlitePath; got != "app/data.db" {
		t.Errorf("RunVolumeBackup sqlitePath = %q, want %q", got, "app/data.db")
	}
}

// TestScheduler_Tick_Volume_FirstSightingArmsWithoutFiring mirrors
// TestScheduler_Tick_FirstSightingArmsWithoutFiring for the volume
// evaluation path (tickServiceVolumes): the same "never fire the instant
// a schedule is first observed" rule applies identically to both.
func TestScheduler_Tick_Volume_FirstSightingArmsWithoutFiring(t *testing.T) {
	now := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	fakeStore := &fakeScheduleStore{volumes: []store.ServiceVolumeBackupConfig{scheduledVolume(0)}}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)
	s.Now = func() time.Time { return now }

	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(runner.volumeCalls) != 0 {
		t.Fatalf("first tick fired %d volume backups, want 0 (arm only)", len(runner.volumeCalls))
	}
}

// TestScheduler_Tick_Volume_FiresOnceDue mirrors
// TestScheduler_Tick_FiresOnceDue for the volume path: after the first
// arming tick, a later due tick actually runs the backup, resolving the
// volume's logical name to its real Docker volume name first.
func TestScheduler_Tick_Volume_FiresOnceDue(t *testing.T) {
	fakeStore := &fakeScheduleStore{
		volumes:           []store.ServiceVolumeBackupConfig{scheduledVolume(0)},
		resolveVolumeName: map[string]string{"web/data": "app-web-data"},
	}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	armAt := time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return armAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("arming Tick() error = %v", err)
	}

	dueAt := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return dueAt }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("due Tick() error = %v", err)
	}

	if len(runner.volumeCalls) != 1 {
		t.Fatalf("volume backup calls = %d, want 1", len(runner.volumeCalls))
	}
	call := runner.volumeCalls[0]
	if call.service != "web" || call.volume != "data" || call.dockerVolume != "app-web-data" || call.targetID != "bkt_1" {
		t.Errorf("volume backup call = %+v, want service=web volume=data dockerVolume=app-web-data targetID=bkt_1", call)
	}
}

// TestScheduler_Tick_Volume_RetentionPrune mirrors the database
// retention-prune behavior: a due volume backup with BackupRetain set
// prunes old backup_history rows through PruneServiceVolumeBackupHistory
// afterward.
func TestScheduler_Tick_Volume_RetentionPrune(t *testing.T) {
	fakeStore := &fakeScheduleStore{
		volumes:           []store.ServiceVolumeBackupConfig{scheduledVolume(5)},
		resolveVolumeName: map[string]string{"web/data": "app-web-data"},
	}
	runner := &fakeScheduledRunner{}
	s := NewScheduler(fakeStore, runner, nil, nil)

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC) }
	_ = s.Tick(context.Background())

	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	if err := s.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	if len(fakeStore.volumePrunedCalls) != 1 {
		t.Fatalf("PruneServiceVolumeBackupHistory calls = %d, want 1", len(fakeStore.volumePrunedCalls))
	}
	call := fakeStore.volumePrunedCalls[0]
	if call.service != "web" || call.volume != "data" || call.keep != 5 {
		t.Errorf("prune call = %+v, want service=web volume=data keep=5", call)
	}
}

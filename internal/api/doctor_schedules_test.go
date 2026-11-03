package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type neverSource struct{ cron string }

func (n neverSource) ListScheduledDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return []store.DesiredDatabase{{Name: "pg", BackupSchedule: n.cron}}, nil
}

func (neverSource) ListScheduledServiceVolumes(context.Context) ([]store.ServiceVolumeBackupConfig, error) {
	return nil, nil
}

func (neverSource) ListEnabledAppSchedules(context.Context) ([]store.AppSchedule, error) {
	return nil, nil
}

func (neverSource) ListEnabledScheduledTasks(context.Context) ([]store.ScheduledTask, error) {
	return nil, nil
}

func TestDoctorCheckScheduleNeverRuns(t *testing.T) {
	t.Parallel()
	rt := &Router{}
	if got := rt.doctorCheckScheduleNeverRuns(context.Background()); got.Status != doctorStatusUnknown {
		t.Errorf("no source: status %v", got.Status)
	}
	rt.SetScheduleScan(neverSource{cron: "0 3 * * *"})
	if got := rt.doctorCheckScheduleNeverRuns(context.Background()); got.Status != doctorStatusOK {
		t.Errorf("valid: status %v", got.Status)
	}
	rt.SetScheduleScan(neverSource{cron: "0 0 30 2 *"})
	got := rt.doctorCheckScheduleNeverRuns(context.Background())
	if got.Status != doctorStatusWarn || got.Fix != "edit or delete the schedule" || got.Evidence == "" {
		t.Errorf("impossible: %+v", got)
	}
}

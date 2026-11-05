package schedulescan

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSource struct{}

func (fakeSource) ListScheduledDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return []store.DesiredDatabase{{Name: "pg", BackupSchedule: "0 0 30 2 *"}, {Name: "ok", BackupSchedule: "0 3 * * *"}}, nil
}

func (fakeSource) ListScheduledServiceVolumes(context.Context) ([]store.ServiceVolumeBackupConfig, error) {
	return []store.ServiceVolumeBackupConfig{{ServiceName: "web", VolumeName: "data", BackupSchedule: "0 0 31 4 *"}}, nil
}

func (fakeSource) ListEnabledAppSchedules(context.Context) ([]store.AppSchedule, error) {
	return []store.AppSchedule{{ServiceName: "api", Cron: "0 0 29 2 *", Timezone: "Europe/Berlin"}, {ServiceName: "once"}}, nil
}

func (fakeSource) ListEnabledScheduledTasks(context.Context) ([]store.ScheduledTask, error) {
	return []store.ScheduledTask{{ID: "t1", ServiceName: "web", Schedule: "0 0 31 6 *"}}, nil
}

func TestScanFindsOnlyNeverRunningSchedules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	got, err := Scan(context.Background(), fakeSource{}, now)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, f := range got {
		ids[f.Kind+":"+f.ID] = true
	}
	for _, want := range []string{"database_backup:pg", "volume_backup:web/data", "scheduled_task:t1"} {
		if !ids[want] {
			t.Errorf("missing finding %s in %v", want, ids)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d findings, want 3 (leap day, daily and one-time must pass): %v", len(got), got)
	}
}

func TestLogWarningsOnePerSchedule(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	LogWarnings(context.Background(), fakeSource{}, slog.New(slog.NewTextHandler(&buf, nil)), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	if n := strings.Count(buf.String(), "stored schedule never runs"); n != 3 {
		t.Fatalf("warn lines = %d, want 3:\n%s", n, buf.String())
	}
}

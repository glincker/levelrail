package main

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestBootEnvIntervalsRejectNonPositive(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		env string
		get func() time.Duration
		def time.Duration
	}{
		{"APP_BACKUP_SCHEDULER_INTERVAL", func() time.Duration { return backupSchedulerInterval(logger) }, defaultBackupSchedulerInterval},
		{"APP_SCHEDULED_TASK_SCHEDULER_INTERVAL", func() time.Duration { return scheduledTaskSchedulerInterval(logger) }, defaultScheduledTaskSchedulerInterval},
		{"APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL", func() time.Duration { return appScheduleSchedulerInterval(logger) }, defaultAppScheduleSchedulerInterval},
		{"APP_UPDATE_CHECK_INTERVAL", func() time.Duration { return updateCheckInterval(logger) }, defaultUpdateCheckInterval},
		{"APP_NODE_HEARTBEAT_INTERVAL", nodeHeartbeatInterval, defaultNodeHeartbeatInterval},
		{"APP_OS_PATCH_CHECK_INTERVAL", osPatchCheckInterval, defaultOSPatchCheckInterval},
	}
	for _, c := range cases {
		for _, raw := range []string{"0s", "-1m", "junk"} {
			t.Setenv(c.env, raw)
			if got := c.get(); got != c.def {
				t.Errorf("%s=%s: got %v, want default %v", c.env, raw, got, c.def)
			}
		}
	}
}

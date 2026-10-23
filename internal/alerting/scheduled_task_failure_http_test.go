package alerting

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestScheduledTaskFailureNotice_Labels(t *testing.T) {
	tests := []struct {
		name string
		task store.ScheduledTask
		want string
	}{
		{"exec shows the command", store.ScheduledTask{Command: []string{"sh", "-c", "x"}, ConsecutiveFailures: 3, LastRunStatus: "failed"}, `"sh -c x" failed 3 runs`},
		{"http shows the job name", store.ScheduledTask{Mode: store.ScheduledTaskModeHTTP, Name: "tick", ConsecutiveFailures: 2, LastRunStatus: "timeout"}, `"tick (HTTP call)" failed 2 runs`},
	}
	for _, tc := range tests {
		if got := scheduledTaskFailureNotice(tc.task); !strings.Contains(got, tc.want) {
			t.Errorf("%s: notice = %q, want it to contain %q", tc.name, got, tc.want)
		}
	}
}

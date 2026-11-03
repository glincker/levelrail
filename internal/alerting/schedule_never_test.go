package alerting

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMaintenanceWindowValidate_RejectsNeverMatching(t *testing.T) {
	w := MaintenanceWindow{Name: "w", Cron: "0 0 31 4 *", Duration: time.Hour, Scope: ScopeAll, Timezone: "Asia/Kolkata"}
	if w.Validate() == nil {
		t.Fatal("impossible window accepted")
	}
	w.Cron = "0 0 29 2 *"
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBackupOverdue_NeverRunningScheduleIsNotOverdue(t *testing.T) {
	hist := []store.BackupHistory{{StartedAt: "2026-01-01T00:00:00Z", Status: store.BackupStatusSucceeded, FinishedAt: "2026-01-01T00:05:00Z"}}
	overdue, _, _, err := BackupOverdue(hist, "0 0 30 2 *", time.Minute, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if overdue || err == nil {
		t.Fatalf("overdue=%v err=%v, want not overdue with an error", overdue, err)
	}
}

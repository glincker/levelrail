package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHasFreshBackup(t *testing.T) {
	now := time.Now().UTC()
	cutoff := now.Add(-time.Hour)
	fresh := now.Add(-time.Minute).Format(time.RFC3339)
	old := now.Add(-2 * time.Hour).Format(time.RFC3339)
	cases := []struct {
		name string
		hist []store.BackupHistory
		want bool
	}{
		{"none", nil, false},
		{"old success", []store.BackupHistory{{Status: store.BackupStatusSucceeded, FinishedAt: old}}, false},
		{"fresh failure", []store.BackupHistory{{Status: store.BackupStatusFailed, FinishedAt: fresh}}, false},
		{"fresh success", []store.BackupHistory{{Status: store.BackupStatusFailed, FinishedAt: fresh}, {Status: store.BackupStatusSucceeded, FinishedAt: fresh}}, true},
	}
	for _, c := range cases {
		if got := hasFreshBackup(c.hist, cutoff); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestTemplateUpgradeRunsSingleFlight(t *testing.T) {
	var runs templateUpgradeRuns
	if !runs.start(templateUpgradeRun{AppID: "a", State: upgradeStateWatching}) {
		t.Fatal("first start refused")
	}
	if runs.start(templateUpgradeRun{AppID: "a", State: upgradeStateWatching}) {
		t.Fatal("second start while watching must be refused")
	}
	runs.update("a", func(r *templateUpgradeRun) { r.State = upgradeStateRolledBack })
	if !runs.start(templateUpgradeRun{AppID: "a", State: upgradeStateWatching}) {
		t.Fatal("start after a finished run must be allowed")
	}
}

func TestTemplateUpgradeSettingsFromEnv(t *testing.T) {
	t.Setenv(envTemplateUpgradeBackupRequired, "false")
	t.Setenv(envTemplateUpgradeCheckInterval, "10m")
	cfg := templateUpgradeSettingsFromEnv()
	if cfg.backupRequired || cfg.recheck != 10*time.Minute {
		t.Fatalf("env not honoured: %+v", cfg)
	}
}

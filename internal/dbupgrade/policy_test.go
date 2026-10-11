package dbupgrade

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPolicyValidate(t *testing.T) {
	base := Policy{AutoUpgrade: store.DBAutoUpgradePatch, WindowCron: "0 3 * * 0", WindowDuration: 2 * time.Hour, WindowTimezone: "UTC", BackupBefore: true, VerifyAfter: true, RevertOnFailure: true}
	tests := []struct {
		name    string
		mutate  func(p *Policy)
		wantErr string
	}{
		{"valid", func(*Policy) {}, ""},
		{"minor allowed", func(p *Policy) { p.AutoUpgrade = store.DBAutoUpgradeMinor }, ""},
		{"major never automatic", func(p *Policy) { p.AutoUpgrade = KindMajor }, "never automatic"},
		{"unknown level", func(p *Policy) { p.AutoUpgrade = "always" }, "auto_upgrade must be"},
		{"backup required", func(p *Policy) { p.BackupBefore = false }, "backup_before"},
		{"window required when on", func(p *Policy) { p.WindowCron, p.WindowDuration = "", 0 }, "window"},
		{"off without window", func(p *Policy) { p.AutoUpgrade, p.WindowCron, p.WindowDuration = store.DBAutoUpgradeOff, "", 0 }, ""},
		{"bad cron", func(p *Policy) { p.WindowCron = "every sunday" }, "cron"},
		{"bad timezone", func(p *Policy) { p.WindowTimezone = "Mars/Olympus" }, "timezone"},
		{"zero duration", func(p *Policy) { p.WindowDuration = 0 }, "duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base
			tt.mutate(&p)
			err := p.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestPolicyCanStartAt(t *testing.T) {
	tests := []struct {
		name     string
		cron     string
		duration time.Duration
		tz       string
		auto     string
		now      string
		want     bool
	}{
		{"inside utc window", "0 3 * * *", 2 * time.Hour, "UTC", store.DBAutoUpgradePatch, "2026-10-10T04:00:00Z", true},
		{"too close to the end", "0 3 * * *", 2 * time.Hour, "UTC", store.DBAutoUpgradePatch, "2026-10-10T04:45:00Z", false},
		{"outside the window", "0 3 * * *", 2 * time.Hour, "UTC", store.DBAutoUpgradePatch, "2026-10-10T06:00:00Z", false},
		{"policy off", "0 3 * * *", 2 * time.Hour, "UTC", store.DBAutoUpgradeOff, "2026-10-10T04:00:00Z", false},
		{"inside new york window", "0 3 * * *", 2 * time.Hour, "America/New_York", store.DBAutoUpgradePatch, "2026-07-01T07:30:00Z", true},
		{"utc 3am is not new york 3am", "0 3 * * *", 2 * time.Hour, "America/New_York", store.DBAutoUpgradePatch, "2026-07-01T03:30:00Z", false},
		{"berlin fall back keeps the window", "0 2 * * *", 2 * time.Hour, "Europe/Berlin", store.DBAutoUpgradePatch, "2026-10-25T01:30:00Z", true},
		{"new york spring forward skips a missing start", "30 2 * * *", time.Hour, "America/New_York", store.DBAutoUpgradePatch, "2026-03-08T07:45:00Z", false},
		{"weekly window on its day", "0 3 * * 0", 2 * time.Hour, "UTC", store.DBAutoUpgradeMinor, "2026-10-11T03:10:00Z", true},
		{"weekly window off its day", "0 3 * * 0", 2 * time.Hour, "UTC", store.DBAutoUpgradeMinor, "2026-10-10T03:10:00Z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Policy{AutoUpgrade: tt.auto, WindowCron: tt.cron, WindowDuration: tt.duration, WindowTimezone: tt.tz, BackupBefore: true}
			now, err := time.Parse(time.RFC3339, tt.now)
			if err != nil {
				t.Fatal(err)
			}
			got, err := p.CanStartAt(now, 30*time.Minute)
			if err != nil {
				t.Fatalf("CanStartAt() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("CanStartAt(%s) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestPolicyWithoutWindowIsNeverOpen(t *testing.T) {
	st, err := Policy{AutoUpgrade: store.DBAutoUpgradePatch}.WindowState(time.Now())
	if err != nil || st.Active || !st.NextStart.IsZero() {
		t.Errorf("WindowState() = %+v, %v, want closed with no next start", st, err)
	}
}

func TestRevertPlan(t *testing.T) {
	tests := []struct {
		name        string
		imageRevert bool
		snapshot    string
		backup      string
		want        []string
	}{
		{"all paths", true, "snap", "bh", []string{PhaseRevertImage, PhaseRevertSnapshot, PhaseRevertBackup}},
		{"engine rewrites data files", false, "snap", "bh", []string{PhaseRevertSnapshot, PhaseRevertBackup}},
		{"no snapshot taken", true, "", "bh", []string{PhaseRevertImage, PhaseRevertBackup}},
		{"backup only", false, "", "bh", []string{PhaseRevertBackup}},
	}
	for _, tt := range tests {
		got := RevertPlan(tt.imageRevert, tt.snapshot, tt.backup)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%s: RevertPlan() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

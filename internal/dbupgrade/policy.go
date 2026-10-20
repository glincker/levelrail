package dbupgrade

import (
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Policy is a database's automatic upgrade policy, or the platform default.
type Policy struct {
	AutoUpgrade     string        `json:"auto_upgrade"`
	WindowCron      string        `json:"window_cron"`
	WindowDuration  time.Duration `json:"-"`
	WindowTimezone  string        `json:"window_timezone"`
	BackupBefore    bool          `json:"backup_before"`
	VerifyAfter     bool          `json:"verify_after"`
	RevertOnFailure bool          `json:"revert_on_failure"`
	Notify          []string      `json:"notify"`
}

// Validate checks a policy before it is saved. A window is required once
// automatic upgrades are on, and a backup before every upgrade is mandatory.
func (p Policy) Validate() error {
	switch p.AutoUpgrade {
	case store.DBAutoUpgradeOff, store.DBAutoUpgradePatch, store.DBAutoUpgradeMinor:
	case KindMajor:
		return errors.New("auto_upgrade cannot be major: major upgrades are never automatic")
	default:
		return fmt.Errorf("auto_upgrade must be %q, %q or %q", store.DBAutoUpgradeOff, store.DBAutoUpgradePatch, store.DBAutoUpgradeMinor)
	}
	if !p.BackupBefore {
		return errors.New("backup_before cannot be turned off: every upgrade starts from a fresh, verified backup")
	}
	if p.WindowCron == "" && p.WindowDuration == 0 {
		if p.AutoUpgrade != store.DBAutoUpgradeOff {
			return errors.New("a maintenance window (window_cron and window_duration) is required when auto_upgrade is on")
		}
		return nil
	}
	if err := p.window().Validate(); err != nil {
		return fmt.Errorf("maintenance window: %w", err)
	}
	return nil
}

func (p Policy) window() alerting.MaintenanceWindow {
	return alerting.MaintenanceWindow{
		Name: "database upgrade window", Cron: p.WindowCron, Duration: p.WindowDuration,
		Timezone: p.WindowTimezone, Scope: alerting.ScopeAll, Enabled: true,
	}
}

// WindowState evaluates the policy's window at now. A policy without a
// window is never open.
func (p Policy) WindowState(now time.Time) (alerting.WindowState, error) {
	if p.WindowCron == "" || p.WindowDuration <= 0 {
		return alerting.WindowState{}, nil
	}
	st, err := p.window().StateAt(now)
	if err != nil {
		return alerting.WindowState{}, fmt.Errorf("dbupgrade: evaluate window: %w", err)
	}
	return st, nil
}

// CanStartAt reports whether an automatic run may start at now: the window
// is open and at least minRemaining of it is left.
func (p Policy) CanStartAt(now time.Time, minRemaining time.Duration) (bool, error) {
	if p.AutoUpgrade == store.DBAutoUpgradeOff {
		return false, nil
	}
	st, err := p.WindowState(now)
	if err != nil || !st.Active {
		return false, err
	}
	return st.End.Sub(now) >= minRemaining, nil
}

// PolicyFromStore converts a stored row.
func PolicyFromStore(s store.DBUpgradePolicy) Policy {
	return Policy{
		AutoUpgrade: s.AutoUpgrade, WindowCron: s.WindowCron, WindowDuration: s.WindowDuration,
		WindowTimezone: s.WindowTimezone, BackupBefore: s.BackupBefore, VerifyAfter: s.VerifyAfter,
		RevertOnFailure: s.RevertOnFailure, Notify: append([]string{}, s.Notify...),
	}
}

// ToStore converts a policy to a row for name.
func (p Policy) ToStore(name, updatedBy string, now time.Time) store.DBUpgradePolicy {
	tz := p.WindowTimezone
	if tz == "" {
		tz = "UTC"
	}
	return store.DBUpgradePolicy{
		DatabaseName: name, AutoUpgrade: p.AutoUpgrade, WindowCron: p.WindowCron, WindowDuration: p.WindowDuration,
		WindowTimezone: tz, BackupBefore: true, VerifyAfter: p.VerifyAfter, RevertOnFailure: p.RevertOnFailure,
		Notify: p.Notify, UpdatedBy: updatedBy, UpdatedAt: now.UTC().Format(time.RFC3339),
	}
}

// DefaultPolicy is used when even the platform default row is unreadable.
func DefaultPolicy() Policy {
	return Policy{AutoUpgrade: store.DBAutoUpgradeOff, WindowTimezone: "UTC", BackupBefore: true, VerifyAfter: true, RevertOnFailure: true, Notify: []string{}}
}

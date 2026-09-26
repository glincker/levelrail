package deploy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MaxFreezeDuration bounds one freeze window so a typo cannot freeze an
// app for months.
const MaxFreezeDuration = 31 * 24 * time.Hour

// ErrFrozen marks a deploy refused because a freeze window is active.
var ErrFrozen = errors.New("deploy frozen")

// FreezeStore lists freeze windows by scope. *store.DB satisfies this.
type FreezeStore interface {
	ListDeployFreezeWindows(ctx context.Context, scopes ...string) ([]store.DeployFreezeWindow, error)
}

// FreezeStatus is whether automatic deploys to a service are held now.
type FreezeStatus struct {
	Frozen bool
	// Until is when the latest-ending active window closes.
	Until    time.Time
	Reason   string
	WindowID string
}

// FrozenError is ErrFrozen with the window that caused it.
type FrozenError struct{ Status FreezeStatus }

func (e *FrozenError) Error() string {
	msg := "deploy frozen until " + e.Status.Until.UTC().Format(time.RFC3339)
	if e.Status.Reason != "" {
		msg += " (" + e.Status.Reason + ")"
	}
	return msg
}

// Is lets errors.Is(err, ErrFrozen) match.
func (e *FrozenError) Is(target error) bool { return target == ErrFrozen }

// ValidateFreezeWindow checks a window before it is stored.
func ValidateFreezeWindow(w store.DeployFreezeWindow) error {
	if _, err := cronexpr.Parse(w.Cron); err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	if w.Duration < time.Minute || w.Duration > MaxFreezeDuration {
		return fmt.Errorf("duration must be between 1m and %s", MaxFreezeDuration)
	}
	if w.Timezone != "" {
		if _, err := time.LoadLocation(w.Timezone); err != nil {
			return fmt.Errorf("timezone: %w", err)
		}
	}
	return nil
}

// WindowActive reports whether w covers now and, if so, when it ends. The
// cron expression is evaluated on the wall clock of w.Timezone.
func WindowActive(w store.DeployFreezeWindow, now time.Time) (bool, time.Time, error) {
	sched, err := cronexpr.Parse(w.Cron)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("freeze window %s: %w", w.ID, err)
	}
	loc := time.UTC
	if w.Timezone != "" {
		if loc, err = time.LoadLocation(w.Timezone); err != nil {
			return false, time.Time{}, fmt.Errorf("freeze window %s: timezone: %w", w.ID, err)
		}
	}
	// cronexpr works in UTC; shift the wall clock of loc into UTC fields.
	local := now.In(loc)
	wall := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
	start := sched.Next(wall.Add(-w.Duration))
	if start.IsZero() || start.After(wall) {
		return false, time.Time{}, nil
	}
	endWall := start.Add(w.Duration)
	end := time.Date(endWall.Year(), endWall.Month(), endWall.Day(), endWall.Hour(), endWall.Minute(), endWall.Second(), 0, loc)
	return true, end, nil
}

// ActiveFreeze folds windows into one status at now. A window that fails to
// evaluate is treated as active: a broken freeze must fail closed.
func ActiveFreeze(windows []store.DeployFreezeWindow, now time.Time) FreezeStatus {
	var st FreezeStatus
	for _, w := range windows {
		active, end, err := WindowActive(w, now)
		if err != nil {
			active, end = true, now.Add(time.Minute)
		}
		if !active {
			continue
		}
		st.Frozen = true
		if end.After(st.Until) {
			st.Until, st.Reason, st.WindowID = end, w.Reason, w.ID
		}
	}
	return st
}

// CheckFreeze returns serviceName's freeze status from its own and the
// global windows.
func CheckFreeze(ctx context.Context, fs FreezeStore, serviceName string, now time.Time) (FreezeStatus, error) {
	if fs == nil {
		return FreezeStatus{}, nil
	}
	windows, err := fs.ListDeployFreezeWindows(ctx, store.DeployFreezeScopeApp(serviceName), store.DeployFreezeScopeGlobal)
	if err != nil {
		return FreezeStatus{}, fmt.Errorf("deploy: check freeze for %q: %w", serviceName, err)
	}
	return ActiveFreeze(windows, now), nil
}

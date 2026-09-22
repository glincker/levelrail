package backup

import (
	"errors"
	"fmt"
	"time"
)

// PITRWindow is one database's currently recoverable point-in-time
// restore range: Start is the oldest succeeded base backup's own
// started_at (nothing earlier is recoverable, since there is no base to
// replay WAL forward from), End is the latest instant WAL archiving has
// continuously and provably reached (RecoverableWindowEnd). HasBaseBackup
// is false whenever no base backup has ever succeeded yet, in which case
// Start/End are both zero and carry no meaning.
type PITRWindow struct {
	HasBaseBackup bool
	Start         time.Time
	End           time.Time
}

// ErrNoBaseBackup is returned by ValidatePITRTarget when win has no
// succeeded base backup at all: point-in-time restore genuinely has
// nothing to restore from yet, not merely a target outside its range.
var ErrNoBaseBackup = errors.New("backup: no succeeded base backup exists yet, point-in-time restore is not available")

// ValidatePITRTarget checks that target actually falls inside win before
// RunPITRRestore ever attempts anything against a real container. This
// is the one guard standing between an operator-chosen timestamp and a
// restore that hangs forever rather than fails cleanly: Postgres recovery
// given a recovery_target_time beyond what's actually archived does not
// error, it sits retrying restore_command for a WAL segment that will
// never arrive (see RecoverableWindowEnd's own doc comment), so this
// function's job is making sure that situation is never reached in the
// first place.
func ValidatePITRTarget(win PITRWindow, target time.Time) error {
	if !win.HasBaseBackup {
		return ErrNoBaseBackup
	}
	if target.Before(win.Start) {
		return fmt.Errorf("backup: target time %s is before the oldest recoverable point %s", target.Format(time.RFC3339), win.Start.Format(time.RFC3339))
	}
	if target.After(win.End) {
		return fmt.Errorf("backup: target time %s is after the latest recoverable point %s", target.Format(time.RFC3339), win.End.Format(time.RFC3339))
	}
	return nil
}

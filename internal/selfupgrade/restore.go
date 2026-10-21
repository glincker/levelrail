package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// StepRestorePoint names the single step of a manual restore.
const StepRestorePoint = "restore_point"

// ErrUpgradeRunning means an upgrade has not finished, so nothing may be restored.
var ErrUpgradeRunning = errors.New("an upgrade is still running or did not finish")

// RestoreFromPoint puts a restore point's binary (and unit file) back and, when
// the live database was migrated past the point's schema, the database copy too.
// It never restores a database that is still compatible, so nothing is lost
// silently. The result is journaled like any attempt.
func RestoreFromPoint(ctx context.Context, d Deps, rp *RestorePoints, id, attemptID, initiator, currentVersion string) (Attempt, error) {
	a := Attempt{ID: attemptID, FromVersion: currentVersion, Initiator: initiator, Outcome: OutcomeRunning, FromSchema: -1, ToSchema: -1, StartedAt: d.now(), Steps: []StepRecord{}}
	r := &runner{d: d, a: &a, healthTimeout: DefaultHealthTimeout}
	pt, err := rp.Get(id)
	if err != nil {
		return r.finishRefused(err)
	}
	a.ToVersion, a.ToSchema, a.SnapshotAt, a.RestorePointID = pt.Version, pt.Schema, pt.CreatedAt, pt.ID
	if d.Journal != nil {
		if running, _ := d.Journal.Running(); len(running) > 0 {
			return r.finishRefused(ErrUpgradeRunning)
		}
	}
	if !pt.HasBinary {
		return r.finishRefused(errors.New("this restore point holds no binary, so it cannot be restored"))
	}
	if cur, err := d.DBSchema(ctx); err == nil {
		a.FromSchema = cur
	}
	d.audit(ctx, "restore point "+pt.ID+" ("+pt.Version+") restore started")
	err = r.step(StepRestorePoint, func() (string, error) {
		if err := d.Stop(ctx); err != nil {
			_ = d.Start(ctx)
			return "", fmt.Errorf("stop service: %w", err)
		}
		if err := swapBinary(rp.BinaryPath(pt.ID), d.BinaryPath); err != nil {
			_ = d.Start(ctx)
			return "", fmt.Errorf("restore binary: %w", err)
		}
		detail := "binary " + pt.Version + " restored"
		if pt.HasUnit && d.RestoreUnit != nil {
			if err := d.RestoreUnit(ctx, rp.UnitPath(pt.ID)); err != nil {
				return "", fmt.Errorf("restore unit file: %w", err)
			}
		}
		if a.FromSchema < 0 || a.FromSchema != pt.Schema {
			kept, err := d.RestoreDB(ctx, rp.DBPath(pt.ID))
			if err != nil {
				return "", fmt.Errorf("restore database: %w", err)
			}
			a.DatabaseRestored = true
			a.RolledBackMinutes = minutesSince(pt.CreatedAt, d.now())
			detail += fmt.Sprintf(", database restored (%d minutes of changes rolled back, replaced database kept as %s)", a.RolledBackMinutes, filepath.Base(kept))
		} else {
			detail += ", database kept: it is already compatible, so no changes were lost"
		}
		if err := d.Start(ctx); err != nil {
			return "", fmt.Errorf("start restored release: %w", err)
		}
		if err := d.WaitHealthy(ctx, DefaultHealthTimeout); err != nil {
			return "", fmt.Errorf("restored release did not become healthy: %w", err)
		}
		return detail + ", release healthy", nil
	})
	if err != nil {
		return r.close(OutcomeFailed, StepRestorePoint, err)
	}
	return r.close(OutcomeRolledBack, "", nil)
}

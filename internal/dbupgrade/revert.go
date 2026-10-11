package dbupgrade

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// RevertPlan lists the revert phases a run will try, cheapest first: the old
// image on the same data, then the volume snapshot, then the logical backup.
// The image step is skipped for engines whose upgrade rewrites data files.
func RevertPlan(imageRevert bool, snapshot, backupID string) []string {
	var plan []string
	if imageRevert {
		plan = append(plan, PhaseRevertImage)
	}
	if snapshot != "" {
		plan = append(plan, PhaseRevertSnapshot)
	}
	if backupID != "" {
		plan = append(plan, PhaseRevertBackup)
	}
	return plan
}

func (r *Runner) imageRevert(engine string) bool {
	if r.Catalog == nil {
		return false
	}
	ec, ok := r.Catalog.Engine(engine)
	return ok && ec.ImageRevert
}

// beginRevert records why the upgrade failed and moves to the first revert
// phase, or fails the run when reverting is off.
func (r *Runner) beginRevert(ctx context.Context, run *store.DBUpgradeRun, reason string) error {
	run.Reason = reason
	if !run.RevertOnFailure {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, reason+"; revert_on_failure is off, so the database was left on "+run.ToVersion)
	}
	return r.nextRevert(ctx, run, "")
}

// nextRevert moves to the revert phase after the current one, or fails the
// run when none is left.
func (r *Runner) nextRevert(ctx context.Context, run *store.DBUpgradeRun, failed string) error {
	plan := RevertPlan(r.imageRevert(run.Engine), run.SnapshotVolume, run.BackupID)
	next := ""
	for i, p := range plan {
		if failed == "" {
			next = p
			break
		}
		if p == failed && i+1 < len(plan) {
			next = plan[i+1]
			break
		}
	}
	if next == "" {
		where := "backup " + run.BackupID
		if run.SnapshotVolume != "" {
			where = fmt.Sprintf("volume %s and backup %s", run.SnapshotVolume, run.BackupID)
		}
		return r.finish(ctx, run, store.DBUpgradeStateFailed, run.Reason+"; REVERT FAILED: the pre-upgrade data is kept in "+where+", restore it by hand")
	}
	run.RevertPath = ""
	return r.enter(ctx, run, store.DBUpgradeStateVerifying, next)
}

// stepRevert applies the current phase's action once (RevertPath marks it
// done), then waits for the old version to report healthy.
func (r *Runner) stepRevert(ctx context.Context, run *store.DBUpgradeRun) error {
	path := map[string]string{
		PhaseRevertImage: RevertPathImage, PhaseRevertSnapshot: RevertPathSnapshot, PhaseRevertBackup: RevertPathBackup,
	}[run.Phase]

	if run.Phase == PhaseRevertBackup {
		return r.revertFromBackup(ctx, run)
	}
	if run.RevertPath != path {
		var err error
		if run.Phase == PhaseRevertImage {
			err = r.Runtime.SetVersion(ctx, run.DatabaseName, run.FromVersion)
		} else {
			err = r.Runtime.RestoreSnapshot(ctx, run.DatabaseName, run.SnapshotVolume, run.FromVersion)
		}
		if err != nil {
			run.Reason += fmt.Sprintf("; %s revert failed: %v", path, err)
			return r.nextRevert(ctx, run, run.Phase)
		}
		run.RevertPath = path
		if err := r.enter(ctx, run, store.DBUpgradeStateVerifying, run.Phase); err != nil {
			return err
		}
	}
	ok, why, err := r.waitHealthy(ctx, run, run.FromVersion, true)
	if err != nil {
		return err
	}
	if !ok {
		run.Reason += fmt.Sprintf("; %s revert did not come up: %s", path, why)
		return r.nextRevert(ctx, run, run.Phase)
	}
	r.dropSnapshot(context.WithoutCancel(ctx), run)
	return r.finish(ctx, run, store.DBUpgradeStateReverted, "")
}

// revertFromBackup rebuilds the old version from the verified logical
// backup. It always starts from an empty volume, so repeating it after a
// restart cannot leave half restored rows behind.
func (r *Runner) revertFromBackup(ctx context.Context, run *store.DBUpgradeRun) error {
	if err := r.Runtime.ResetData(ctx, run.DatabaseName, run.FromVersion); err != nil {
		run.Reason += "; resetting the data volume for a restore failed: " + err.Error()
		return r.nextRevert(ctx, run, run.Phase)
	}
	if err := r.enter(ctx, run, store.DBUpgradeStateVerifying, run.Phase); err != nil {
		return err
	}
	ok, why, err := r.waitHealthy(ctx, run, run.FromVersion, true)
	if err != nil {
		return err
	}
	if !ok {
		run.Reason += "; the old version did not start on an empty volume: " + why
		return r.nextRevert(ctx, run, run.Phase)
	}
	db, err := r.Store.GetDesiredDatabase(ctx, run.DatabaseName)
	if err != nil {
		return fmt.Errorf("dbupgrade: load database %q: %w", run.DatabaseName, err)
	}
	if err := r.Runtime.RestoreBackup(ctx, newID("rh_"), run.BackupID, *db); err != nil {
		run.Reason += "; restoring backup " + run.BackupID + " failed: " + err.Error()
		return r.nextRevert(ctx, run, run.Phase)
	}
	run.RevertPath = RevertPathBackup
	return r.finish(ctx, run, store.DBUpgradeStateReverted, "")
}

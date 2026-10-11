package dbupgrade

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Phases inside a state. Persisted, so a restarted control plane resumes
// at the step it was on.
const (
	PhaseBackup         = "backup"
	PhaseVerifyBackup   = "verify_backup"
	PhasePreflight      = "preflight"
	PhaseSnapshot       = "snapshot"
	PhaseBump           = "bump"
	PhaseHealth         = "health"
	PhaseRevertImage    = "revert_image"
	PhaseRevertSnapshot = "revert_snapshot"
	PhaseRevertBackup   = "revert_backup"
)

// Revert paths recorded on a run so the history says how it was undone.
const (
	RevertPathImage    = "image"
	RevertPathSnapshot = "volume_snapshot"
	RevertPathBackup   = "backup_restore"
)

// Store is the persistence the runner and manager need; *store.DB satisfies it.
type Store interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	CreateDBUpgradeRun(ctx context.Context, r store.DBUpgradeRun) error
	UpdateDBUpgradeRun(ctx context.Context, r store.DBUpgradeRun) error
	GetDBUpgradeRun(ctx context.Context, id string) (store.DBUpgradeRun, error)
	ListDBUpgradeRuns(ctx context.Context, databaseName string, limit int) ([]store.DBUpgradeRun, error)
	ListActiveDBUpgradeRuns(ctx context.Context) ([]store.DBUpgradeRun, error)
	GetDBUpgradePolicy(ctx context.Context, name string) (store.DBUpgradePolicy, bool, error)
	SaveDBUpgradePolicy(ctx context.Context, p store.DBUpgradePolicy) error
	DeleteDBUpgradePolicy(ctx context.Context, name string) error
}

// Runtime is every side effect a run has on the outside world. Each call
// is safe to repeat after a restart.
type Runtime interface {
	// Busy returns a non-empty reason when another job owns the database.
	Busy(ctx context.Context, name string) (string, error)
	RunBackup(ctx context.Context, backupID string, db store.DesiredDatabase) error
	BackupStatus(ctx context.Context, backupID string) (string, error)
	RunVerify(ctx context.Context, verificationID, backupID, engine string) error
	VerifyStatus(ctx context.Context, backupID, verificationID string) (string, error)
	CheckImage(ctx context.Context, engine, version string) error
	ImageDigest(ctx context.Context, name string) (string, error)
	// SnapshotData stops the database and copies its data volume into volume; it stays stopped.
	SnapshotData(ctx context.Context, name, volume string) error
	// SetVersion saves the desired version and makes sure the database runs.
	SetVersion(ctx context.Context, name, version string) error
	Resume(ctx context.Context, name string) error
	// Health reports whether the database runs version and, when deep, answers
	// a query. It returns the server's reported version when it has one.
	Health(ctx context.Context, db store.DesiredDatabase, version string, deep bool) (ready bool, serverVersion string, err error)
	RestoreSnapshot(ctx context.Context, name, volume, version string) error
	// ResetData empties the data volume and starts version on it fresh.
	ResetData(ctx context.Context, name, version string) error
	RestoreBackup(ctx context.Context, restoreID, backupID string, db store.DesiredDatabase) error
	RemoveVolume(ctx context.Context, volume string) error
}

// Notifier sends a plain text message to notification channels.
type Notifier interface {
	Notify(ctx context.Context, channelIDs []string, text string) error
}

// Runner advances one upgrade run through its state machine.
type Runner struct {
	Store    Store
	Runtime  Runtime
	Catalog  *Catalog
	Notifier Notifier
	Logger   *slog.Logger
	Now      func() time.Time
	// HealthTimeout bounds each wait for the database to come up healthy.
	HealthTimeout time.Duration
	Poll          time.Duration
}

// Drive advances run id until it reaches a final state or ctx ends.
func (r *Runner) Drive(ctx context.Context, id string) error {
	for first := true; ; first = false {
		run, err := r.Store.GetDBUpgradeRun(ctx, id)
		if err != nil {
			return fmt.Errorf("dbupgrade: load run %q: %w", id, err)
		}
		if store.DBUpgradeTerminal(run.State) {
			return nil
		}
		// A resumed health wait gets a full timeout: the reconciler may not
		// have restarted the container yet after a control plane restart.
		if first && run.State == store.DBUpgradeStateVerifying {
			if err := r.enter(ctx, &run, run.State, run.Phase); err != nil {
				return err
			}
		}
		if err := r.step(ctx, &run); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (r *Runner) step(ctx context.Context, run *store.DBUpgradeRun) error {
	switch run.State {
	case store.DBUpgradeStatePending:
		return r.stepPending(ctx, run)
	case store.DBUpgradeStateBackingUp:
		return r.stepBackup(ctx, run)
	case store.DBUpgradeStateUpgrading:
		return r.stepUpgrade(ctx, run)
	case store.DBUpgradeStateVerifying:
		return r.stepVerify(ctx, run)
	}
	return r.finish(ctx, run, store.DBUpgradeStateFailed, "unknown state "+run.State)
}

func (r *Runner) stepPending(ctx context.Context, run *store.DBUpgradeRun) error {
	reason, err := r.Runtime.Busy(ctx, run.DatabaseName)
	if err != nil {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, "could not check for other jobs: "+err.Error())
	}
	if reason != "" {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, "not started: "+reason)
	}
	r.notify(ctx, run, fmt.Sprintf("Database %s: upgrade from %s to %s started (backup first).", run.DatabaseName, run.FromVersion, run.ToVersion))
	return r.enter(ctx, run, store.DBUpgradeStateBackingUp, PhaseBackup)
}

func (r *Runner) stepBackup(ctx context.Context, run *store.DBUpgradeRun) error {
	db, err := r.Store.GetDesiredDatabase(ctx, run.DatabaseName)
	if err != nil {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, "load database: "+err.Error())
	}
	if db.BackupTargetID == "" {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, "no backup target is configured for this database; a fresh backup is required before every upgrade")
	}
	if run.Phase == PhaseVerifyBackup {
		return r.stepVerifyBackup(ctx, run, db)
	}
	if run.BackupID != "" {
		st, err := r.Runtime.BackupStatus(ctx, run.BackupID)
		if err != nil {
			return r.finish(ctx, run, store.DBUpgradeStateFailed, "read backup status: "+err.Error())
		}
		switch st {
		case store.BackupStatusSucceeded:
			return r.enter(ctx, run, store.DBUpgradeStateBackingUp, PhaseVerifyBackup)
		case store.BackupStatusFailed:
			return r.finish(ctx, run, store.DBUpgradeStateFailed, "the pre-upgrade backup "+run.BackupID+" failed; nothing was changed")
		}
		// Running or missing: the process that owned it died, so take a new one.
	}
	run.BackupID = newID("bh_")
	if err := r.save(ctx, run); err != nil {
		return err
	}
	if err := r.Runtime.RunBackup(ctx, run.BackupID, *db); err != nil {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, "the pre-upgrade backup failed, nothing was changed: "+err.Error())
	}
	return nil
}

func (r *Runner) stepVerifyBackup(ctx context.Context, run *store.DBUpgradeRun, db *store.DesiredDatabase) error {
	if run.VerificationID != "" {
		st, err := r.Runtime.VerifyStatus(ctx, run.BackupID, run.VerificationID)
		if err != nil {
			return r.finish(ctx, run, store.DBUpgradeStateFailed, "read backup verification: "+err.Error())
		}
		switch st {
		case store.BackupVerificationStatusPassed:
			return r.enter(ctx, run, store.DBUpgradeStateUpgrading, PhasePreflight)
		case store.BackupVerificationStatusFailed:
			return r.finish(ctx, run, store.DBUpgradeStateFailed, "the pre-upgrade backup "+run.BackupID+" failed verification; nothing was changed")
		}
	}
	run.VerificationID = newID("bv_")
	if err := r.save(ctx, run); err != nil {
		return err
	}
	if err := r.Runtime.RunVerify(ctx, run.VerificationID, run.BackupID, db.Engine); err != nil {
		return r.finish(ctx, run, store.DBUpgradeStateFailed, "verifying the pre-upgrade backup failed, nothing was changed: "+err.Error())
	}
	return nil
}

func (r *Runner) stepUpgrade(ctx context.Context, run *store.DBUpgradeRun) error {
	switch run.Phase {
	case PhasePreflight:
		db, err := r.Store.GetDesiredDatabase(ctx, run.DatabaseName)
		if err != nil {
			return r.finish(ctx, run, store.DBUpgradeStateFailed, "load database: "+err.Error())
		}
		if db.Version != run.FromVersion {
			return r.finish(ctx, run, store.DBUpgradeStateFailed, fmt.Sprintf("the database version changed to %s while the run waited; nothing was changed", db.Version))
		}
		if err := database.CheckInPlaceVersionChange(run.Engine, run.FromVersion, run.ToVersion); err != nil {
			return r.finish(ctx, run, store.DBUpgradeStateFailed, err.Error())
		}
		if err := r.Runtime.CheckImage(ctx, run.Engine, run.ToVersion); err != nil {
			return r.finish(ctx, run, store.DBUpgradeStateFailed, "the target image is not available, nothing was changed: "+err.Error())
		}
		digest, err := r.Runtime.ImageDigest(ctx, run.DatabaseName)
		if err != nil {
			r.log().Warn("dbupgrade: could not record the pre-upgrade image", slog.String("run", run.ID), slog.String("error", err.Error()))
		}
		run.FromImageDigest = digest
		next := PhaseBump
		if run.RevertOnFailure {
			next = PhaseSnapshot
		}
		return r.enter(ctx, run, store.DBUpgradeStateUpgrading, next)
	case PhaseSnapshot:
		run.SnapshotVolume = fmt.Sprintf("db-%s-data-preupgrade-%s", run.DatabaseName, strings.TrimPrefix(run.ID, "dbu_"))
		if err := r.save(ctx, run); err != nil {
			return err
		}
		if err := r.Runtime.SnapshotData(ctx, run.DatabaseName, run.SnapshotVolume); err != nil {
			return r.abortBeforeBump(ctx, run, "snapshot of the data volume failed: "+err.Error())
		}
		return r.enter(ctx, run, store.DBUpgradeStateUpgrading, PhaseBump)
	default:
		if err := r.Runtime.SetVersion(ctx, run.DatabaseName, run.ToVersion); err != nil {
			return r.beginRevert(ctx, run, "applying version "+run.ToVersion+" failed: "+err.Error())
		}
		return r.enter(ctx, run, store.DBUpgradeStateVerifying, PhaseHealth)
	}
}

// abortBeforeBump undoes a run that stopped the database but never changed
// its version: start it again and drop the partial snapshot.
func (r *Runner) abortBeforeBump(ctx context.Context, run *store.DBUpgradeRun, reason string) error {
	bg := context.WithoutCancel(ctx)
	if err := r.Runtime.Resume(bg, run.DatabaseName); err != nil {
		reason += "; restarting the database also failed: " + err.Error()
	}
	r.dropSnapshot(bg, run)
	return r.finish(ctx, run, store.DBUpgradeStateFailed, reason+"; the version was not changed")
}

func (r *Runner) stepVerify(ctx context.Context, run *store.DBUpgradeRun) error {
	switch run.Phase {
	case PhaseRevertImage, PhaseRevertSnapshot, PhaseRevertBackup:
		return r.stepRevert(ctx, run)
	}
	ok, why, err := r.waitHealthy(ctx, run, run.ToVersion, run.VerifyAfter)
	if err != nil {
		return err
	}
	if ok {
		r.dropSnapshot(context.WithoutCancel(ctx), run)
		return r.finish(ctx, run, store.DBUpgradeStateSucceeded, "")
	}
	return r.beginRevert(ctx, run, why)
}

// waitHealthy polls Health until it passes or HealthTimeout from the phase
// start runs out. ok=false with a reason means the deadline passed.
func (r *Runner) waitHealthy(ctx context.Context, run *store.DBUpgradeRun, version string, deep bool) (bool, string, error) {
	started, err := time.Parse(time.RFC3339, run.PhaseStartedAt)
	if err != nil {
		started = r.now()
	}
	deadline := started.Add(r.healthTimeout())
	last := "the database did not report healthy"
	for {
		db, err := r.Store.GetDesiredDatabase(ctx, run.DatabaseName)
		if err != nil {
			return false, "", fmt.Errorf("dbupgrade: load database %q: %w", run.DatabaseName, err)
		}
		ready, server, herr := r.Runtime.Health(ctx, *db, version, deep)
		switch {
		case herr != nil:
			last = herr.Error()
		case ready && deep && server != "" && !serverMatches(server, version):
			last = fmt.Sprintf("the server reports version %s, expected %s", server, version)
		case ready:
			return true, "", nil
		}
		if !r.now().Before(deadline) {
			return false, fmt.Sprintf("health check on %s failed within %s: %s", version, r.healthTimeout(), last), nil
		}
		if err := sleepCtx(ctx, r.poll()); err != nil {
			return false, "", err
		}
	}
}

// serverMatches compares each numeric component the expected tag pins.
func serverMatches(server, expected string) bool {
	sv, ok := parseVersion(strings.Fields(server + " ")[0])
	ev, ok2 := parseVersion(expected)
	if !ok || !ok2 {
		return true
	}
	for i, n := range ev.nums {
		if at(sv.nums, i) != n {
			return false
		}
	}
	return true
}

func (r *Runner) enter(ctx context.Context, run *store.DBUpgradeRun, state, phase string) error {
	now := r.now().UTC().Format(time.RFC3339)
	run.State, run.Phase, run.PhaseStartedAt = state, phase, now
	if run.Timings == nil {
		run.Timings = map[string]string{}
	}
	run.Timings[state+"."+phase] = now
	return r.save(ctx, run)
}

func (r *Runner) finish(ctx context.Context, run *store.DBUpgradeRun, state, reason string) error {
	// A shutdown mid-step is not a failure: leave the run where it was so the
	// next control plane resumes it.
	if err := ctx.Err(); err != nil {
		return err
	}
	now := r.now().UTC().Format(time.RFC3339)
	run.State, run.FinishedAt = state, now
	if reason != "" {
		run.Reason = reason
	}
	if run.Timings == nil {
		run.Timings = map[string]string{}
	}
	run.Timings[state] = now
	if err := r.save(context.WithoutCancel(ctx), run); err != nil {
		return err
	}
	level := slog.LevelInfo
	if state != store.DBUpgradeStateSucceeded {
		level = slog.LevelWarn
	}
	r.log().Log(ctx, level, "dbupgrade: run finished", slog.String("run", run.ID), slog.String("database", run.DatabaseName),
		slog.String("state", state), slog.String("from", run.FromVersion), slog.String("to", run.ToVersion),
		slog.String("revert_path", run.RevertPath), slog.String("reason", run.Reason))
	r.notify(ctx, run, finishText(run))
	return nil
}

func finishText(run *store.DBUpgradeRun) string {
	switch run.State {
	case store.DBUpgradeStateSucceeded:
		return fmt.Sprintf("Database %s upgraded from %s to %s and passed its health checks. Pre-upgrade backup: %s.", run.DatabaseName, run.FromVersion, run.ToVersion, run.BackupID)
	case store.DBUpgradeStateReverted:
		return fmt.Sprintf("Database %s upgrade to %s failed and was reverted to %s (path: %s). Reason: %s", run.DatabaseName, run.ToVersion, run.FromVersion, run.RevertPath, run.Reason)
	default:
		return fmt.Sprintf("Database %s upgrade from %s to %s failed: %s", run.DatabaseName, run.FromVersion, run.ToVersion, run.Reason)
	}
}

func (r *Runner) notify(ctx context.Context, run *store.DBUpgradeRun, text string) {
	if r.Notifier == nil || len(run.Notify) == 0 {
		return
	}
	if err := r.Notifier.Notify(context.WithoutCancel(ctx), run.Notify, text); err != nil {
		r.log().Warn("dbupgrade: notification failed", slog.String("run", run.ID), slog.String("error", err.Error()))
	}
}

func (r *Runner) save(ctx context.Context, run *store.DBUpgradeRun) error {
	if err := r.Store.UpdateDBUpgradeRun(ctx, *run); err != nil {
		return fmt.Errorf("dbupgrade: save run %q: %w", run.ID, err)
	}
	return nil
}

func (r *Runner) dropSnapshot(ctx context.Context, run *store.DBUpgradeRun) {
	if run.SnapshotVolume == "" {
		return
	}
	if err := r.Runtime.RemoveVolume(ctx, run.SnapshotVolume); err != nil {
		r.log().Warn("dbupgrade: snapshot volume left behind", slog.String("volume", run.SnapshotVolume), slog.String("error", err.Error()))
	}
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) healthTimeout() time.Duration {
	if r.HealthTimeout > 0 {
		return r.HealthTimeout
	}
	return DefaultHealthTimeout
}

func (r *Runner) poll() time.Duration {
	if r.Poll > 0 {
		return r.Poll
	}
	return 2 * time.Second
}

func (r *Runner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

// DefaultHealthTimeout bounds each wait for a healthy database; override with APP_DB_UPGRADE_HEALTH_TIMEOUT.
const DefaultHealthTimeout = 5 * time.Minute

func newID(prefix string) string {
	return prefix + strings.ToLower(rand.Text()[:18])
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

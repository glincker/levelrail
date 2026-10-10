package rollback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// Outcomes of Apply.
const (
	OutcomeApplied   = "applied"
	OutcomeRecovered = "recovered"
)

// Refusals from Apply. Each is raised before anything is changed.
var (
	ErrConfirmVersion     = errors.New("typed confirmation does not match the target version")
	ErrNotRetained        = errors.New("target release is not retained on this host")
	ErrSchemaNewer        = errors.New("database schema is newer than the target release supports")
	ErrSchemaUnknown      = errors.New("target release schema version is unknown")
	ErrRestoreNotNeeded   = errors.New("a backup restore is not needed for this target")
	ErrBackupIncompatible = errors.New("backup is not compatible with the target release")
	ErrConfirmDataLoss    = errors.New("data loss confirmation does not match the backup name")
	ErrRecovered          = errors.New("target release failed to start, previous release restored")
)

// Deps are the host operations Apply drives. Tests fake them; the CLI wires
// real ones (systemd, sqlite, filesystem).
type Deps struct {
	BinaryPath  string
	ReleasesDir string
	BinaryName  string
	// DBSchema reads the live database's schema version.
	DBSchema func(ctx context.Context) (int, error)
	// Backups lists restore points with their schema versions.
	Backups func(ctx context.Context) ([]BackupOption, error)
	// Backup takes a fresh backup of the live database and returns its name.
	Backup func(ctx context.Context) (string, error)
	Stop   func(ctx context.Context) error
	Start  func(ctx context.Context) error
	// WaitHealthy returns nil once the control plane answers its health and
	// readiness probes, or an error when timeout passes first.
	WaitHealthy func(ctx context.Context, timeout time.Duration) error
	// RestoreDB swaps the named backup in and returns the path the previous
	// database was kept at. UndoRestoreDB puts that path back.
	RestoreDB     func(ctx context.Context, backupName string) (string, error)
	UndoRestoreDB func(ctx context.Context, keptPath string) error
	// Audit records the outcome in the audit log. Best effort.
	Audit func(ctx context.Context, detail string) error
	Log   *slog.Logger
	Now   func() time.Time
}

// Options are the operator's choices for one rollback.
type Options struct {
	CurrentVersion  string
	ConfirmVersion  string
	RestoreBackup   string
	ConfirmDataLoss string
	AcceptUnknown   bool
	HealthTimeout   time.Duration
}

// Result reports what Apply did.
type Result struct {
	Outcome     string `json:"outcome"`
	From        string `json:"from"`
	To          string `json:"to"`
	BackupName  string `json:"backup_name,omitempty"`
	Restored    string `json:"restored_backup,omitempty"`
	KeptDB      string `json:"previous_database,omitempty"`
	Plan        Plan   `json:"plan"`
	HealthError string `json:"health_error,omitempty"`
}

// Guard decides whether opts may proceed against plan, before any change.
func Guard(plan Plan, opts Options) error {
	if opts.ConfirmVersion != plan.Target.Version {
		return fmt.Errorf("%w: type %s to confirm", ErrConfirmVersion, plan.Target.Version)
	}
	switch plan.Verdict {
	case upgrade.VerdictRestoreRequired:
		if opts.RestoreBackup == "" {
			return fmt.Errorf("%w: database is at schema %d, %s supports %d. A binary-only rollback would not start. Restoring a backup is possible but loses data written after it: re-run with --restore-backup <name> --confirm-data-loss <name>",
				ErrSchemaNewer, plan.CurrentSchemaVersion, plan.Target.Version, plan.Target.SchemaVersion)
		}
		var chosen *BackupOption
		for i := range plan.Backups {
			if plan.Backups[i].Name == opts.RestoreBackup {
				chosen = &plan.Backups[i]
			}
		}
		if chosen == nil {
			return fmt.Errorf("%w: no backup named %q", ErrBackupIncompatible, opts.RestoreBackup)
		}
		if !chosen.Compatible {
			return fmt.Errorf("%w: %s is at schema %d, %s supports %d", ErrBackupIncompatible, chosen.Name, chosen.SchemaVersion, plan.Target.Version, plan.Target.SchemaVersion)
		}
		if opts.ConfirmDataLoss != chosen.Name {
			return fmt.Errorf("%w: restoring %s (taken %s) loses everything written since; type its name with --confirm-data-loss", ErrConfirmDataLoss, chosen.Name, chosen.CreatedAt.UTC().Format(time.RFC3339))
		}
	case upgrade.VerdictUnknown:
		if !opts.AcceptUnknown {
			return fmt.Errorf("%w: %s publishes no schema information. The older binary refuses to start on a newer schema without touching data, and the previous release is restored automatically; pass --accept-unknown-schema to try", ErrSchemaUnknown, plan.Target.Version)
		}
		fallthrough
	default:
		if opts.RestoreBackup != "" {
			return ErrRestoreNotNeeded
		}
	}
	return nil
}

// Apply performs the rollback: guard, fresh backup, optional restore, binary
// swap, restart, health verification, and automatic recovery. Nothing changes
// until Guard and the target checksum pass.
func Apply(ctx context.Context, d Deps, plan Plan, opts Options) (Result, error) {
	res := Result{From: opts.CurrentVersion, To: plan.Target.Version, Plan: plan}
	if err := Guard(plan, opts); err != nil {
		return res, err
	}
	target, ok, err := Find(d.ReleasesDir, d.BinaryName, plan.Target.Version)
	if err != nil {
		return res, err
	}
	if !ok {
		return res, fmt.Errorf("%w: %s", ErrNotRetained, plan.Target.Version)
	}
	if err := Verify(target); err != nil {
		return res, err
	}

	backup, err := d.Backup(ctx)
	if err != nil {
		return res, fmt.Errorf("pre-rollback backup failed, nothing changed: %w", err)
	}
	res.BackupName = backup
	d.Log.Info("rollback backup taken", slog.String("backup", backup), slog.String("from", res.From), slog.String("to", res.To))

	prev := d.BinaryPath + ".prev"
	if err := copyExecutable(d.BinaryPath, prev); err != nil {
		return res, fmt.Errorf("keep previous binary, nothing changed: %w", err)
	}
	keepPrev := false
	defer func() {
		if !keepPrev {
			_ = os.Remove(prev)
		}
	}()

	d.audit(ctx, "rollback "+res.From+" -> "+res.To+" started, backup "+backup)
	if err := d.Stop(ctx); err != nil {
		return res, fmt.Errorf("stop control plane, nothing changed: %w", err)
	}
	var keptDB string
	if plan.Verdict == upgrade.VerdictRestoreRequired {
		keptDB, err = d.RestoreDB(ctx, opts.RestoreBackup)
		if err != nil {
			_ = d.Start(ctx)
			return res, fmt.Errorf("restore backup %s, previous binary and database untouched: %w", opts.RestoreBackup, err)
		}
		res.Restored, res.KeptDB = opts.RestoreBackup, keptDB
	}
	if err := swapBinary(target.Path, d.BinaryPath); err != nil {
		d.recover(ctx, prev, keptDB)
		return res, fmt.Errorf("install target binary, previous release restored: %w", err)
	}
	startErr := d.Start(ctx)
	if startErr == nil {
		startErr = d.WaitHealthy(ctx, opts.HealthTimeout)
	}
	if startErr != nil {
		res.HealthError = startErr.Error()
		d.Log.Error("rolled-back release failed to start, restoring previous", slog.String("to", res.To), slog.String("error", startErr.Error()))
		if err := d.recoverAndVerify(ctx, prev, keptDB, opts.HealthTimeout); err != nil {
			keepPrev = true
			return res, fmt.Errorf("target failed to start (%v) and recovery also failed: %w. Previous binary is at %s, previous database at %q", startErr, err, prev, keptDB)
		}
		res.Outcome = OutcomeRecovered
		d.audit(ctx, "rollback "+res.From+" -> "+res.To+" failed to boot, previous release restored")
		return res, fmt.Errorf("%w: %v", ErrRecovered, startErr)
	}
	res.Outcome = OutcomeApplied
	d.audit(ctx, "rollback "+res.From+" -> "+res.To+" applied, backup "+backup)
	return res, nil
}

func (d Deps) audit(ctx context.Context, detail string) {
	if d.Audit == nil {
		return
	}
	if err := d.Audit(ctx, detail); err != nil {
		d.Log.Warn("rollback audit entry failed", slog.String("error", err.Error()))
	}
}

func swapBinary(src, dst string) error {
	tmp := dst + ".new"
	if err := copyExecutable(src, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (d Deps) recover(ctx context.Context, prev, keptDB string) {
	if err := swapBinary(prev, d.BinaryPath); err != nil {
		d.Log.Error("restore previous binary failed", slog.String("error", err.Error()))
	}
	if keptDB != "" {
		if err := d.UndoRestoreDB(ctx, keptDB); err != nil {
			d.Log.Error("restore previous database failed", slog.String("error", err.Error()))
		}
	}
	if err := d.Start(ctx); err != nil {
		d.Log.Error("start previous release failed", slog.String("error", err.Error()))
	}
}

func (d Deps) recoverAndVerify(ctx context.Context, prev, keptDB string, timeout time.Duration) error {
	if err := d.Stop(ctx); err != nil {
		d.Log.Warn("stop failed target before recovery", slog.String("error", err.Error()))
	}
	if err := swapBinary(prev, d.BinaryPath); err != nil {
		return fmt.Errorf("restore previous binary: %w", err)
	}
	if keptDB != "" {
		if err := d.UndoRestoreDB(ctx, keptDB); err != nil {
			return fmt.Errorf("restore previous database: %w", err)
		}
	}
	if err := d.Start(ctx); err != nil {
		return fmt.Errorf("start previous release: %w", err)
	}
	if err := d.WaitHealthy(ctx, timeout); err != nil {
		return fmt.Errorf("previous release did not become healthy: %w", err)
	}
	return nil
}

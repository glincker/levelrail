package selfupgrade

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type runner struct {
	d             Deps
	a             *Attempt
	healthTimeout time.Duration
}

// step runs fn, records it on the timeline and journals the attempt before
// returning, so a crash right after leaves the journal at the right step.
func (r *runner) step(name string, fn func() (string, error)) error {
	start := r.d.now()
	detail, err := fn()
	rec := StepRecord{Name: name, Status: StatusOK, Detail: detail, At: start, DurationMS: r.d.now().Sub(start).Milliseconds()}
	if err != nil {
		rec.Status, rec.Detail = StatusFailed, err.Error()
	}
	r.a.Steps = append(r.a.Steps, rec)
	r.save()
	return err
}

func (r *runner) save() {
	if r.d.Journal == nil {
		return
	}
	if err := r.d.Journal.Save(*r.a); err != nil {
		r.d.log().Warn("self-upgrade journal write failed", slog.String("attempt", r.a.ID), slog.String("error", err.Error()))
	}
}

func (r *runner) close(outcome, step string, err error) (Attempt, error) {
	r.a.Outcome, r.a.FailedStep, r.a.FinishedAt = outcome, step, r.d.now()
	if err != nil {
		r.a.Error = err.Error()
	}
	r.save()
	r.d.audit(context.Background(), fmt.Sprintf("self-upgrade %s -> %s %s at %s: %v", r.a.FromVersion, r.a.ToVersion, outcome, step, err))
	return *r.a, err
}

// finishRefused closes an attempt that was stopped by a check before any
// change to the host.
func (r *runner) finishRefused(err error) (Attempt, error) {
	_ = os.RemoveAll(r.d.WorkDir)
	return r.close(OutcomeRefused, r.a.lastStep(), err)
}

// cleanFail closes an attempt that failed before the stop step: the service
// kept running the old binary and nothing was replaced.
func (r *runner) cleanFail(step string, err error) (Attempt, error) {
	_ = os.RemoveAll(r.d.WorkDir)
	if r.a.PrevBinary != "" {
		_ = os.Remove(r.a.PrevBinary)
	}
	return r.close(OutcomeFailed, step, err)
}

// rollbackAndFinish puts the previous binary back, restores the snapshot when
// the new release changed the schema, and verifies the old release is healthy.
func (r *runner) rollbackAndFinish(ctx context.Context, failedStep string, cause error) (Attempt, error) {
	d, a := r.d, r.a
	d.log().Error("upgrade failed, rolling back", slog.String("attempt", a.ID), slog.String("step", failedStep), slog.String("error", cause.Error()))
	// The caller's context may be what was cancelled; rollback must still run.
	ctx = context.WithoutCancel(ctx)
	if d.ClearMarker != nil {
		d.ClearMarker()
	}
	timeout := r.healthTimeout
	if timeout <= 0 {
		timeout = DefaultHealthTimeout
	}
	err := r.step(StepRollback, func() (string, error) {
		_ = d.Stop(ctx)
		if a.PrevBinary != "" {
			if err := swapBinary(a.PrevBinary, d.BinaryPath); err != nil {
				return "", fmt.Errorf("restore previous binary: %w", err)
			}
		}
		detail := "previous binary restored"
		if r.schemaChanged(ctx) && a.SnapshotPath != "" {
			kept, err := d.RestoreDB(ctx, a.SnapshotPath)
			if err != nil {
				return "", fmt.Errorf("restore database snapshot: %w", err)
			}
			detail += ", database restored from " + a.BackupName + " (failed run kept at " + kept + ")"
		}
		if err := d.Start(ctx); err != nil {
			return "", fmt.Errorf("start previous release: %w", err)
		}
		if err := d.WaitHealthy(ctx, timeout); err != nil {
			return "", fmt.Errorf("previous release did not become healthy: %w", err)
		}
		return detail + ", previous release healthy", nil
	})
	if err != nil {
		return r.close(OutcomeFailed, failedStep, fmt.Errorf("%v, and rollback failed: %w. Previous binary: %s, snapshot: %s", cause, err, a.PrevBinary, a.SnapshotPath))
	}
	if a.PrevBinary != "" {
		_ = os.Remove(a.PrevBinary)
	}
	return r.close(OutcomeRolledBack, failedStep, fmt.Errorf("%w: rolled back to %s", cause, a.FromVersion))
}

// schemaChanged reports whether the live database no longer matches the
// schema the attempt started from, which the previous binary cannot open.
// An unreadable database is treated as changed.
func (r *runner) schemaChanged(ctx context.Context) bool {
	if r.a.FromSchema < 0 || r.d.DBSchema == nil {
		return true
	}
	cur, err := r.d.DBSchema(ctx)
	return err != nil || cur != r.a.FromSchema
}

func swapBinary(src, dst string) error {
	tmp := dst + ".new"
	if err := copyExecutable(src, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // paths come from the upgrade's own work and install directories
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(src), err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gosec // executable by design
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(dst), err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync %s: %w", filepath.Base(dst), err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(dst), err)
	}
	return nil
}

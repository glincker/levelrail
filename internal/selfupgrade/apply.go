package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/kit/semver"
)

// DefaultHealthTimeout bounds how long a new release may take to become
// healthy before the previous binary and data are put back.
const DefaultHealthTimeout = 90 * time.Second

// ProbeInfo is what a downloaded binary reports about itself.
type ProbeInfo struct {
	Version string
	Schema  int
}

// BackupInfo names the pre-upgrade restore point.
type BackupInfo struct {
	Name         string
	SnapshotPath string
}

// Deps are the host operations Apply drives. Tests fake them; the host
// command wires real ones (HTTPS, cosign, systemd, sqlite, filesystem).
type Deps struct {
	BinaryPath string
	WorkDir    string
	Asset      string
	Journal    *Journal
	Log        *slog.Logger
	Now        func() time.Time

	Fetch       func(ctx context.Context, tag, asset, dir string) (Fetched, error)
	VerifySig   func(ctx context.Context, checksumsPath, bundlePath string) (string, error)
	Probe       func(ctx context.Context, binary string) (ProbeInfo, error)
	DBSchema    func(ctx context.Context) (int, error)
	Backup      func(ctx context.Context) (BackupInfo, error)
	MigrateTest func(ctx context.Context, binary, snapshotPath string) (int, error)
	Stop        func(ctx context.Context) error
	Start       func(ctx context.Context) error
	WaitHealthy func(ctx context.Context, timeout time.Duration) error
	// RestoreDB puts the snapshot in place of the live database and returns
	// where the replaced database was kept.
	RestoreDB   func(ctx context.Context, snapshotPath string) (string, error)
	WriteMarker func(ctx context.Context, a Attempt) error
	ClearMarker func()
	Audit       func(ctx context.Context, detail string) error
}

// Request is one upgrade the operator asked for.
type Request struct {
	ID             string
	CurrentVersion string
	TargetVersion  string
	Initiator      string
	Breaking       []Breaking
	Acked          []string
	HealthTimeout  time.Duration
}

// Apply runs the upgrade: guard, download, verify, back up, migration dry
// run, swap, restart, health check, and rollback on failure. Nothing on the
// host changes before the stop step. err is nil only on success.
func Apply(ctx context.Context, d Deps, req Request) (Attempt, error) {
	if req.HealthTimeout <= 0 {
		req.HealthTimeout = DefaultHealthTimeout
	}
	a := Attempt{
		ID: req.ID, FromVersion: req.CurrentVersion, ToVersion: req.TargetVersion, Initiator: req.Initiator,
		Outcome: OutcomeRunning, Acked: req.Acked, FromSchema: -1, ToSchema: -1, StartedAt: d.now(), Steps: []StepRecord{},
	}
	r := &runner{d: d, a: &a, healthTimeout: req.HealthTimeout}
	if err := r.step(StepGuard, func() (string, error) { return guard(req) }); err != nil {
		return r.finishRefused(err)
	}
	if schema, err := d.DBSchema(ctx); err == nil {
		a.FromSchema = schema
	}
	var fetched Fetched
	if err := r.step(StepDownload, func() (string, error) {
		var err error
		fetched, err = d.Fetch(ctx, req.TargetVersion, d.Asset, d.WorkDir)
		return d.Asset + " " + req.TargetVersion, err
	}); err != nil {
		return r.cleanFail(StepDownload, err)
	}
	defer func() { _ = os.RemoveAll(d.WorkDir) }()
	if err := r.step(StepChecksum, func() (string, error) {
		return "sha256 matches checksums.txt", VerifyChecksum(fetched.BinaryPath, fetched.ChecksumsPath, d.Asset)
	}); err != nil {
		return r.finishRefused(err)
	}
	if err := r.step(StepSignature, func() (string, error) {
		return d.VerifySig(ctx, fetched.ChecksumsPath, fetched.BundlePath)
	}); err != nil {
		return r.finishRefused(err)
	}
	var probe ProbeInfo
	if err := r.step(StepProbe, func() (string, error) {
		var err error
		probe, err = d.Probe(ctx, fetched.BinaryPath)
		if err != nil {
			return "", err
		}
		a.ToSchema = probe.Schema
		if probe.Version != req.TargetVersion {
			return "", fmt.Errorf("%w: asked for %s, binary says %s", ErrVersionMismatch, req.TargetVersion, probe.Version)
		}
		if a.FromSchema > probe.Schema {
			return "", fmt.Errorf("%w: database is at schema %d, %s supports %d. Use rollback to go back to an older release", ErrDowngrade, a.FromSchema, probe.Version, probe.Schema)
		}
		return fmt.Sprintf("%s, schema %d", probe.Version, probe.Schema), nil
	}); err != nil {
		return r.finishRefused(err)
	}
	var backup BackupInfo
	if err := r.step(StepBackup, func() (string, error) {
		var err error
		backup, err = d.Backup(ctx)
		if err != nil {
			return "", err
		}
		a.BackupName, a.SnapshotPath = backup.Name, backup.SnapshotPath
		a.PrevBinary = d.BinaryPath + ".prev"
		if err := copyExecutable(d.BinaryPath, a.PrevBinary); err != nil {
			return "", fmt.Errorf("keep previous binary: %w", err)
		}
		return "database snapshot " + backup.Name + ", previous binary kept", nil
	}); err != nil {
		return r.cleanFail(StepBackup, err)
	}
	if err := r.step(StepMigrationCheck, func() (string, error) {
		schema, err := d.MigrateTest(ctx, fetched.BinaryPath, backup.SnapshotPath)
		if errors.Is(err, ErrDryRunUnsupported) {
			return "skipped: target predates the migration dry-run, relying on the snapshot and automatic rollback", nil
		}
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrMigrationCheck, err)
		}
		return fmt.Sprintf("snapshot migrates from %d to %d", a.FromSchema, schema), nil
	}); err != nil {
		return r.cleanFail(StepMigrationCheck, err)
	}

	d.audit(ctx, "self-upgrade "+a.FromVersion+" -> "+a.ToVersion+" started, backup "+backup.Name)
	if err := r.step(StepStop, func() (string, error) { return "", d.Stop(ctx) }); err != nil {
		_ = d.Start(ctx)
		return r.cleanFail(StepStop, err)
	}
	if err := r.step(StepSwap, func() (string, error) {
		if err := swapBinary(fetched.BinaryPath, d.BinaryPath); err != nil {
			return "", err
		}
		a.SwappedBinary = true
		if d.WriteMarker != nil {
			if err := d.WriteMarker(ctx, a); err != nil {
				d.log().Warn("upgrade context marker not written", slog.String("attempt", a.ID), slog.String("error", err.Error()))
			}
		}
		return "binary replaced", nil
	}); err != nil {
		return r.rollbackAndFinish(ctx, StepSwap, err)
	}
	if err := r.step(StepStart, func() (string, error) { return "", d.Start(ctx) }); err != nil {
		return r.rollbackAndFinish(ctx, StepStart, err)
	}
	if err := r.step(StepHealth, func() (string, error) {
		if err := d.WaitHealthy(ctx, req.HealthTimeout); err != nil {
			return "", fmt.Errorf("%w: %v", ErrNotHealthy, err)
		}
		return "healthz and readyz answered", nil
	}); err != nil {
		return r.rollbackAndFinish(ctx, StepHealth, err)
	}
	a.Outcome, a.FinishedAt = OutcomeSucceeded, d.now()
	r.save()
	d.audit(ctx, "self-upgrade "+a.FromVersion+" -> "+a.ToVersion+" succeeded")
	_ = os.Remove(a.PrevBinary)
	return a, nil
}

// Recover finishes an attempt whose host command died mid-way: closes one that
// never stopped the service, rolls back one that did, confirms a healthy one.
func Recover(ctx context.Context, d Deps, a Attempt, healthTimeout time.Duration) (Attempt, error) {
	if healthTimeout <= 0 {
		healthTimeout = DefaultHealthTimeout
	}
	r := &runner{d: d, a: &a, healthTimeout: healthTimeout}
	stopped := false
	healthy := false
	for _, s := range a.Steps {
		if s.Name == StepStop && s.Status == StatusOK {
			stopped = true
		}
		if s.Name == StepHealth && s.Status == StatusOK {
			healthy = true
		}
	}
	interrupted := errors.New("host command was interrupted")
	switch {
	case healthy:
		a.Outcome, a.FinishedAt = OutcomeSucceeded, d.now()
		r.save()
		return a, nil
	case !stopped:
		_ = d.Start(ctx)
		a.Steps = append(a.Steps, StepRecord{Name: StepRollback, Status: StatusSkipped, Detail: "interrupted before the service was stopped, nothing to undo", At: d.now()})
		a.Outcome, a.FailedStep, a.Error, a.FinishedAt = OutcomeFailed, a.lastStep(), interrupted.Error(), d.now()
		r.save()
		return a, interrupted
	}
	return r.rollbackAndFinish(ctx, a.lastStep(), interrupted)
}

func guard(req Request) (string, error) {
	if !ValidTag(req.TargetVersion) {
		return "", fmt.Errorf("refusing unsafe release tag %q", req.TargetVersion)
	}
	if cmp, ok := semver.Compare(req.TargetVersion, req.CurrentVersion); ok {
		switch {
		case cmp == 0:
			return "", fmt.Errorf("already running %s", req.CurrentVersion)
		case cmp < 0:
			return "", fmt.Errorf("%w: %s is older than the running %s. Use rollback to return to an older release", ErrDowngrade, req.TargetVersion, req.CurrentVersion)
		}
	}
	if missing := MissingAcks(req.Breaking, req.Acked); len(missing) > 0 {
		return "", AckError(missing)
	}
	return fmt.Sprintf("%s to %s, %d breaking change(s) acknowledged", req.CurrentVersion, req.TargetVersion, len(req.Breaking)), nil
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d Deps) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (d Deps) audit(ctx context.Context, detail string) {
	if d.Audit == nil {
		return
	}
	if err := d.Audit(ctx, detail); err != nil {
		d.log().Warn("self-upgrade audit entry failed", slog.String("error", err.Error()))
	}
}

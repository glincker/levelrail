package dbrotate

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Typed failures the API maps to status codes.
var (
	ErrUnsupported  = errors.New("this engine has no owner password to rotate")
	ErrNoSecrets    = errors.New("secrets are not configured on this control plane")
	ErrNotRunning   = errors.New("database has no running container")
	ErrPending      = errors.New("a previous rotation is not settled yet")
	ErrVerifyFailed = errors.New("new password was refused by the engine, the old one was restored")
	ErrBadInterval  = errors.New("interval must be 0 to 3650 days")
)

// Reasons stored on a rotation row. Fixed strings, never engine output.
const (
	ReasonVerifyFailed = "verify_failed"
	ReasonApplyFailed  = "apply_failed"
	ReasonRefUpdate    = "reference_update_failed"
	ReasonUnresolved   = "unresolved"
	ReasonInterrupted  = "interrupted"

	maxIntervalDays = 3650
	pendingSuffix   = "_pending"
	passwordBytes   = 24
	sweepBatch      = 5

	ActionStart    = "database.password.rotate.start"
	ActionComplete = "database.password.rotate"
	ActionRollback = "database.password.rotate.rollback"
	ActionPolicy   = "database.password.policy"
	SystemActor    = "database-password-rotation"
)

// Store is the rotation persistence. *store.DB satisfies it.
type Store interface {
	GetDatabasePasswordRotation(ctx context.Context, name string) (store.DatabasePasswordRotation, error)
	ListDatabasePasswordRotations(ctx context.Context) ([]store.DatabasePasswordRotation, error)
	SetDatabasePasswordRotationInterval(ctx context.Context, name string, days int, now time.Time) error
	SaveDatabasePasswordRotationState(ctx context.Context, name, state, reason string, lastRotated, pendingSince, now time.Time) error
	DeleteDatabasePasswordRotation(ctx context.Context, name string) error
}

// Databases reads desired database records.
type Databases interface {
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
}

// Secrets is the encrypted credential store.
type Secrets interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	SetValueGuarded(ctx context.Context, serviceName, envKey, plaintext string, overwriteLocked bool) error
	ListKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error)
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// Target finds the running container of a database. It returns ErrNotRunning
// when there is none.
type Target interface {
	Locate(ctx context.Context, d *store.DesiredDatabase) (Exec, string, error)
}

// Apps lists and restarts the apps that read a database credential.
type Apps interface {
	// Dependents splits linked apps into running ones, which restart, and
	// suspended ones, which pick the new credential up when started.
	Dependents(ctx context.Context, database string) (running, suspended []string, err error)
	Restart(ctx context.Context, database string) []string
}

// Auditor writes one audit row. It is never handed a password.
type Auditor interface {
	Record(ctx context.Context, action, database, actor string)
}

// Service rotates database owner passwords.
type Service struct {
	Store     Store
	Databases Databases
	Secrets   Secrets
	Target    Target
	Apps      Apps
	Auditor   Auditor
	Logger    *slog.Logger
	Now       func() time.Time

	locks sync.Map
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Service) audit(ctx context.Context, action, db, actor string) {
	if s.Auditor != nil {
		s.Auditor.Record(ctx, action, db, actor)
	}
}

func (s *Service) lock(name string) func() {
	v, _ := s.locks.LoadOrStore(name, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Preview is what an operator sees before confirming a rotation.
type Preview struct {
	Database        string
	Engine          string
	Supported       bool
	Reason          string
	RestartRequired bool
	Apps            []string
	SuspendedApps   []string
	State           string
	LastError       string
	LastChangedAt   time.Time
	IntervalDays    int
	NextDue         time.Time
}

// Result is the outcome of one completed rotation.
type Result struct {
	Database      string
	RestartedApps []string
	RotatedAt     time.Time
}

// Preview reports what rotating name would do, without changing anything.
func (s *Service) Preview(ctx context.Context, name string) (Preview, error) {
	d, err := s.Databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		return Preview{}, fmt.Errorf("load database %q: %w", name, err)
	}
	rec, err := s.Store.GetDatabasePasswordRotation(ctx, name)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{
		Database: name, Engine: d.Engine, State: rec.State, LastError: rec.LastError,
		IntervalDays: rec.IntervalDays, LastChangedAt: rec.LastRotatedAt,
	}
	if p.State == "" {
		p.State = store.DBPasswordRotationIdle
	}
	if _, ok := DriverFor(d.Engine); !ok {
		p.Reason = ErrUnsupported.Error()
		return p, nil
	}
	p.Supported = true
	if key, ok := database.PasswordSecretKey(d.Engine); ok && s.Secrets != nil {
		if keys, kerr := s.Secrets.ListKeys(ctx, name); kerr == nil {
			for _, k := range keys {
				if k.Key == key && k.UpdatedAt.After(p.LastChangedAt) {
					p.LastChangedAt = k.UpdatedAt
				}
			}
		}
	}
	p.NextDue = secretrotation.NextDue(s.basis(rec), rec.IntervalDays)
	if s.Apps != nil {
		run, susp, aerr := s.Apps.Dependents(ctx, name)
		if aerr != nil {
			return Preview{}, aerr
		}
		p.Apps, p.SuspendedApps = run, susp
		p.RestartRequired = len(run) > 0
	}
	return p, nil
}

func (s *Service) basis(rec store.DatabasePasswordRotation) time.Time {
	return secretrotation.Basis(rec.LastRotatedAt, rec.CreatedAt)
}

type target struct {
	name       string
	key        string
	pendingKey string
	desired    *store.DesiredDatabase
	driver     Driver
	ex         Exec
	cid        string
}

func (s *Service) resolveTarget(ctx context.Context, name string) (*target, error) {
	if s.Secrets == nil {
		return nil, ErrNoSecrets
	}
	d, err := s.Databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("load database %q: %w", name, err)
	}
	drv, ok := DriverFor(d.Engine)
	if !ok {
		return nil, ErrUnsupported
	}
	key, _ := database.PasswordSecretKey(d.Engine)
	ex, cid, err := s.Target.Locate(ctx, d)
	if err != nil {
		return nil, err
	}
	return &target{name: name, key: key, pendingKey: key + pendingSuffix, desired: d, driver: drv, ex: ex, cid: cid}, nil
}

func newPassword() (string, error) {
	buf := make([]byte, passwordBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("dbrotate: generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Rotate performs one staged rotation: stage the new password, apply it in the
// engine, verify a real login, switch the stored credential, then restart the
// linked apps. A failed verification restores the old password.
func (s *Service) Rotate(ctx context.Context, name, actor string) (Result, error) {
	defer s.lock(name)()
	t, err := s.resolveTarget(ctx, name)
	if err != nil {
		return Result{}, err
	}
	rec, err := s.Store.GetDatabasePasswordRotation(ctx, name)
	if err != nil {
		return Result{}, err
	}
	if rec.State == store.DBPasswordRotationPending {
		res, serr := s.settle(ctx, t, actor)
		if serr != nil {
			return Result{}, serr
		}
		if res.committed {
			return res.result, nil
		}
	}
	old, err := s.Secrets.Resolve(ctx, name, t.key)
	if err != nil {
		return Result{}, fmt.Errorf("resolve current password for %q: %w", name, err)
	}
	if old == "" {
		return Result{}, fmt.Errorf("database %q has no stored password yet", name)
	}
	fresh, err := newPassword()
	if err != nil {
		return Result{}, err
	}
	if err := s.Secrets.SetValueGuarded(ctx, name, t.pendingKey, fresh, true); err != nil {
		return Result{}, fmt.Errorf("stage new password for %q: %w", name, err)
	}
	now := s.now()
	if err := s.Store.SaveDatabasePasswordRotationState(ctx, name, store.DBPasswordRotationPending, "", time.Time{}, now, now); err != nil {
		s.dropPending(ctx, t)
		return Result{}, err
	}
	s.audit(ctx, ActionStart, name, actor)

	if err := t.driver.Set(ctx, t.ex, t.cid, name, old, fresh); err != nil {
		s.log().Warn("dbrotate: apply failed, settling", slog.String("database", name), slog.String("error", err.Error()))
		res, serr := s.settle(ctx, t, actor)
		if serr != nil {
			return Result{}, serr
		}
		if res.committed {
			return res.result, nil
		}
		s.mark(ctx, name, store.DBPasswordRotationFailed, ReasonApplyFailed)
		return Result{}, fmt.Errorf("apply new password to %q: %w", name, err)
	}
	if err := t.driver.Verify(ctx, t.ex, t.cid, name, fresh); err != nil {
		s.log().Warn("dbrotate: new password failed verification, restoring", slog.String("database", name), slog.String("error", err.Error()))
		return Result{}, s.rollback(ctx, t, old, fresh, actor)
	}
	return s.commit(ctx, t, fresh, actor)
}

func (s *Service) rollback(ctx context.Context, t *target, old, fresh, actor string) error {
	if err := t.driver.Set(ctx, t.ex, t.cid, t.name, fresh, old); err != nil {
		s.mark(ctx, t.name, store.DBPasswordRotationPending, ReasonUnresolved)
		return fmt.Errorf("restore old password on %q failed, will settle on the next pass: %w", t.name, err)
	}
	if err := t.driver.Verify(ctx, t.ex, t.cid, t.name, old); err != nil {
		s.mark(ctx, t.name, store.DBPasswordRotationPending, ReasonUnresolved)
		return fmt.Errorf("old password on %q did not verify after restore, will settle on the next pass: %w", t.name, err)
	}
	s.dropPending(ctx, t)
	s.mark(ctx, t.name, store.DBPasswordRotationFailed, ReasonVerifyFailed)
	s.audit(ctx, ActionRollback, t.name, actor)
	return ErrVerifyFailed
}

// commit switches the stored credential to fresh. A failure leaves the row
// pending, so the next pass finds fresh verifying and completes the switch.
func (s *Service) commit(ctx context.Context, t *target, fresh, actor string) (Result, error) {
	if err := s.Secrets.SetValueGuarded(ctx, t.name, t.key, fresh, true); err != nil {
		s.mark(ctx, t.name, store.DBPasswordRotationPending, ReasonRefUpdate)
		return Result{}, fmt.Errorf("switch stored credential for %q, will settle on the next pass: %w", t.name, err)
	}
	s.dropPending(ctx, t)
	now := s.now()
	if err := s.Store.SaveDatabasePasswordRotationState(ctx, t.name, store.DBPasswordRotationIdle, "", now, time.Time{}, now); err != nil {
		s.log().Warn("dbrotate: record rotation failed", slog.String("database", t.name), slog.String("error", err.Error()))
	}
	var restarted []string
	if s.Apps != nil {
		restarted = s.Apps.Restart(ctx, t.name)
	}
	s.audit(ctx, ActionComplete, t.name, actor)
	s.log().Info("dbrotate: database password rotated", slog.String("database", t.name), slog.Int("restarted_apps", len(restarted)))
	return Result{Database: t.name, RestartedApps: restarted, RotatedAt: now}, nil
}

func (s *Service) dropPending(ctx context.Context, t *target) {
	if err := s.Secrets.DeleteValue(ctx, t.name, t.pendingKey); err != nil {
		s.log().Warn("dbrotate: clear staged password failed", slog.String("database", t.name), slog.String("error", err.Error()))
	}
}

func (s *Service) mark(ctx context.Context, name, state, reason string) {
	now := s.now()
	var pendingSince time.Time
	if state == store.DBPasswordRotationPending {
		pendingSince = now
	}
	if err := s.Store.SaveDatabasePasswordRotationState(ctx, name, state, reason, time.Time{}, pendingSince, now); err != nil {
		s.log().Warn("dbrotate: record state failed", slog.String("database", name), slog.String("error", err.Error()))
	}
}

type settleResult struct {
	committed bool
	settled   bool
	result    Result
}

// settle finishes an unsettled rotation by asking the engine which password it
// holds: the staged one completes the switch, the stored one clears the stage.
func (s *Service) settle(ctx context.Context, t *target, actor string) (settleResult, error) {
	staged, err := s.Secrets.Resolve(ctx, t.name, t.pendingKey)
	if err != nil || staged == "" {
		s.mark(ctx, t.name, store.DBPasswordRotationIdle, "")
		return settleResult{settled: true}, nil
	}
	if t.driver.Verify(ctx, t.ex, t.cid, t.name, staged) == nil {
		res, cerr := s.commit(ctx, t, staged, actor)
		if cerr != nil {
			return settleResult{}, cerr
		}
		return settleResult{committed: true, settled: true, result: res}, nil
	}
	current, err := s.Secrets.Resolve(ctx, t.name, t.key)
	if err == nil && current != "" && t.driver.Verify(ctx, t.ex, t.cid, t.name, current) == nil {
		s.dropPending(ctx, t)
		s.mark(ctx, t.name, store.DBPasswordRotationFailed, ReasonInterrupted)
		s.audit(ctx, ActionRollback, t.name, actor)
		return settleResult{settled: true}, nil
	}
	s.mark(ctx, t.name, store.DBPasswordRotationPending, ReasonUnresolved)
	return settleResult{}, fmt.Errorf("%w: neither password verifies on %q", ErrPending, t.name)
}

// Heal settles name's unsettled rotation. A row that is not pending is left alone.
func (s *Service) Heal(ctx context.Context, name string) error {
	defer s.lock(name)()
	rec, err := s.Store.GetDatabasePasswordRotation(ctx, name)
	if err != nil {
		return err
	}
	if rec.State != store.DBPasswordRotationPending {
		return nil
	}
	t, err := s.resolveTarget(ctx, name)
	if err != nil {
		return err
	}
	_, err = s.settle(ctx, t, SystemActor)
	return err
}

// SetInterval sets how often name rotates on its own, in days (0 is off).
func (s *Service) SetInterval(ctx context.Context, name string, days int, actor string) error {
	if days < 0 || days > maxIntervalDays {
		return ErrBadInterval
	}
	d, err := s.Databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		return fmt.Errorf("load database %q: %w", name, err)
	}
	if _, ok := DriverFor(d.Engine); !ok && days > 0 {
		return ErrUnsupported
	}
	if err := s.Store.SetDatabasePasswordRotationInterval(ctx, name, days, s.now()); err != nil {
		return err
	}
	s.audit(ctx, ActionPolicy, name, actor)
	return nil
}

// Sweep settles pending rotations and rotates databases whose interval has
// elapsed, at most sweepBatch rotations per pass. It returns how many it rotated.
func (s *Service) Sweep(ctx context.Context) int {
	rows, err := s.Store.ListDatabasePasswordRotations(ctx)
	if err != nil {
		s.log().Error("dbrotate: list rotations failed", slog.String("error", err.Error()))
		return 0
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Database < rows[j].Database })
	now := s.now()
	rotated := 0
	for _, rec := range rows {
		if _, err := s.Databases.GetDesiredDatabase(ctx, rec.Database); errors.Is(err, store.ErrDatabaseNotFound) {
			_ = s.Store.DeleteDatabasePasswordRotation(ctx, rec.Database)
			continue
		}
		switch {
		case rec.State == store.DBPasswordRotationPending:
			if err := s.Heal(ctx, rec.Database); err != nil {
				s.log().Warn("dbrotate: settle failed, will retry", slog.String("database", rec.Database), slog.String("error", err.Error()))
			}
		case rec.IntervalDays > 0 && rotated < sweepBatch:
			due := secretrotation.NextDue(s.basis(rec), rec.IntervalDays)
			if due.IsZero() || now.Before(due) {
				continue
			}
			if _, err := s.Rotate(ctx, rec.Database, SystemActor); err != nil {
				s.log().Warn("dbrotate: scheduled rotation failed", slog.String("database", rec.Database), slog.String("error", err.Error()))
				continue
			}
			rotated++
		}
	}
	return rotated
}

// Run sweeps once at boot, then every interval until ctx ends.
func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	loop := footprint.Track("database credential rotation sweep", interval)
	defer loop.Active()()
	sweep := func() {
		start := time.Now()
		s.Sweep(ctx)
		loop.Observe(start, nil)
	}
	sweep()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			sweep()
		}
	}
}

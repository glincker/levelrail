package secretrotation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Reasons stored on a policy. Fixed strings only: an error from a restart or
// a store call is logged, never persisted, so no value can leak into state.
const (
	ReasonLocked        = "the secret is locked, unlock it to allow rotation"
	ReasonMissing       = "the secret or its app no longer exists"
	ReasonPreviousInUse = "the grace key is already used by another secret"
	ReasonWriteFailed   = "writing the new value failed, nothing was changed"
	ReasonRestartFailed = "the new value is stored but restarting the app failed, retry the restart"

	ActionRotate        = "secret.rotate"
	ActionRestartFailed = "secret.rotate.restart_failed"
	ActionRotateFailed  = "secret.rotate.failed"
	ActionGraceExpired  = "secret.rotate.grace_expired"

	secretByteLen = 32
	systemActor   = "system"
)

// ErrNotAuto is returned when rotation is requested for a policy that is
// not marked auto-rotatable.
var ErrNotAuto = errors.New("secretrotation: policy is not auto-rotatable")

// Store is the policy persistence the engine needs.
type Store interface {
	GetSecretRotationPolicy(ctx context.Context, service, key string) (store.SecretRotationPolicy, error)
	ListSecretRotationPolicies(ctx context.Context, service string) ([]store.SecretRotationPolicy, error)
	SaveSecretRotationProgress(ctx context.Context, p store.SecretRotationPolicy, now time.Time) error
	DeleteSecretRotationPolicy(ctx context.Context, service, key string) (bool, error)
}

// Backend is the secret and app surface rotation acts through.
type Backend interface {
	// AppState reports whether the app exists and whether it is stopped.
	AppState(ctx context.Context, app string) (exists, suspended bool, err error)
	// Value reads a secret in plaintext. Only the engine's rotate path calls it.
	Value(ctx context.Context, app, key string) (string, error)
	SetValue(ctx context.Context, app, key, value string) error
	DeleteValue(ctx context.Context, app, key string) error
	// KeyState reports existence, lock state and last write time without a value.
	KeyState(ctx context.Context, app, key string) (exists, locked bool, updatedAt time.Time, err error)
	// Restart recreates the app through the normal health-gated cutover.
	Restart(ctx context.Context, app string) error
}

// Auditor records one rotation event. It is never handed a value.
type Auditor interface {
	Record(ctx context.Context, action, app, key, actor string)
}

// Engine runs rotation policies.
type Engine struct {
	Store   Store
	Backend Backend
	Audit   Auditor
	Logger  *slog.Logger
	Now     func() time.Time
	// MaxPerPass bounds scheduled rotations per pass so restarts spread out.
	MaxPerPass int
	mu         sync.Mutex
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) log() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

func (e *Engine) audit(ctx context.Context, action, app, key, actor string) {
	if e.Audit != nil {
		e.Audit.Record(ctx, action, app, key, actor)
	}
}

func generateValue() (string, error) {
	b := make([]byte, secretByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("secretrotation: generate value: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Rotate performs one rotation of a policy's secret, as actor. A failure
// before the new value is written leaves every value untouched. A failure
// after leaves the policy in restart_failed so the next pass retries only
// the restart and the failure stays visible.
func (e *Engine) Rotate(ctx context.Context, app, key, actor string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.Store.GetSecretRotationPolicy(ctx, app, key)
	if err != nil {
		return fmt.Errorf("secretrotation: load policy: %w", err)
	}
	return e.rotateLocked(ctx, p, actor)
}

func (e *Engine) fail(ctx context.Context, p store.SecretRotationPolicy, reason, actor string) error {
	p.State, p.LastError = store.SecretRotationFailed, reason
	if err := e.Store.SaveSecretRotationProgress(ctx, p, e.now()); err != nil {
		e.log().Error("secret rotation: record failure", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
	}
	e.audit(ctx, ActionRotateFailed, p.ServiceName, p.EnvKey, actor)
	return errors.New("secretrotation: " + reason)
}

func (e *Engine) rotateLocked(ctx context.Context, p store.SecretRotationPolicy, actor string) error {
	if !p.AutoRotate {
		return ErrNotAuto
	}
	if p.State == store.SecretRotationRestartFailed || p.State == store.SecretRotationPending {
		return e.retryRestart(ctx, p, actor)
	}
	return e.apply(ctx, p, actor, "", true)
}

// RotateValue writes a new value for a secret (value, or a generated one
// when empty) without restarting the app, so the caller can offer a redeploy.
// The policy must already exist; it carries the grace window and progress.
func (e *Engine) RotateValue(ctx context.Context, app, key, value, actor string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, err := e.Store.GetSecretRotationPolicy(ctx, app, key)
	if err != nil {
		return fmt.Errorf("secretrotation: load policy: %w", err)
	}
	return e.apply(ctx, p, actor, value, false)
}

func (e *Engine) apply(ctx context.Context, p store.SecretRotationPolicy, actor, value string, restart bool) error {
	exists, locked, _, err := e.Backend.KeyState(ctx, p.ServiceName, p.EnvKey)
	if err != nil {
		return fmt.Errorf("secretrotation: read key state: %w", err)
	}
	if !exists {
		return e.fail(ctx, p, ReasonMissing, actor)
	}
	if locked {
		return e.fail(ctx, p, ReasonLocked, actor)
	}
	prevKey := PreviousKey(p.EnvKey)
	keepPrevious := p.GraceHours > 0
	if keepPrevious {
		prevExists, _, _, perr := e.Backend.KeyState(ctx, p.ServiceName, prevKey)
		if perr != nil {
			return fmt.Errorf("secretrotation: read grace key state: %w", perr)
		}
		if prevExists && p.PreviousExpiresAt.IsZero() {
			return e.fail(ctx, p, ReasonPreviousInUse, actor)
		}
	}
	old, err := e.Backend.Value(ctx, p.ServiceName, p.EnvKey)
	if err != nil {
		return e.fail(ctx, p, ReasonMissing, actor)
	}
	next := value
	if next == "" {
		if next, err = generateValue(); err != nil {
			return err
		}
	}
	if keepPrevious {
		if err := e.Backend.SetValue(ctx, p.ServiceName, prevKey, old); err != nil {
			e.log().Error("secret rotation: store previous value", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
			return e.fail(ctx, p, ReasonWriteFailed, actor)
		}
	}
	// Progress is saved before the new value lands so a crash between the
	// two leaves a retryable restart, never a changed value nobody tracks.
	now := e.now()
	pending := p
	pending.LastRotatedAt = now
	pending.State, pending.LastError = store.SecretRotationPending, ""
	pending.PreviousExpiresAt = time.Time{}
	if keepPrevious {
		pending.PreviousExpiresAt = now.Add(time.Duration(p.GraceHours) * time.Hour)
	}
	if err := e.Store.SaveSecretRotationProgress(ctx, pending, now); err != nil {
		e.cleanupPrevious(ctx, p, keepPrevious)
		return fmt.Errorf("secretrotation: save progress: %w", err)
	}
	if err := e.Backend.SetValue(ctx, p.ServiceName, p.EnvKey, next); err != nil {
		e.log().Error("secret rotation: write new value", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
		e.cleanupPrevious(ctx, p, keepPrevious)
		return e.fail(ctx, p, ReasonWriteFailed, actor)
	}
	e.audit(ctx, ActionRotate, p.ServiceName, p.EnvKey, actor)
	if restart {
		return e.retryRestart(ctx, pending, actor)
	}
	pending.State = store.SecretRotationIdle
	if err := e.Store.SaveSecretRotationProgress(ctx, pending, e.now()); err != nil {
		return fmt.Errorf("secretrotation: save progress: %w", err)
	}
	return nil
}

func (e *Engine) cleanupPrevious(ctx context.Context, p store.SecretRotationPolicy, kept bool) {
	if !kept {
		return
	}
	if err := e.Backend.DeleteValue(ctx, p.ServiceName, PreviousKey(p.EnvKey)); err != nil {
		e.log().Warn("secret rotation: remove unused previous value", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
	}
}

func (e *Engine) retryRestart(ctx context.Context, p store.SecretRotationPolicy, actor string) error {
	exists, suspended, err := e.Backend.AppState(ctx, p.ServiceName)
	if err != nil {
		return e.restartFailed(ctx, p, actor, err)
	}
	if !exists {
		return e.fail(ctx, p, ReasonMissing, actor)
	}
	if !suspended {
		if err := e.Backend.Restart(ctx, p.ServiceName); err != nil {
			return e.restartFailed(ctx, p, actor, err)
		}
	}
	p.State, p.LastError = store.SecretRotationIdle, ""
	if err := e.Store.SaveSecretRotationProgress(ctx, p, e.now()); err != nil {
		return fmt.Errorf("secretrotation: save progress: %w", err)
	}
	return nil
}

func (e *Engine) restartFailed(ctx context.Context, p store.SecretRotationPolicy, actor string, cause error) error {
	e.log().Error("secret rotation: restart failed", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", cause.Error()))
	p.State, p.LastError = store.SecretRotationRestartFailed, ReasonRestartFailed
	if err := e.Store.SaveSecretRotationProgress(ctx, p, e.now()); err != nil {
		e.log().Error("secret rotation: record restart failure", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
	}
	e.audit(ctx, ActionRestartFailed, p.ServiceName, p.EnvKey, actor)
	return fmt.Errorf("secretrotation: %s: %w", ReasonRestartFailed, cause)
}

// Sweep runs one pass: retry stuck restarts, expire grace values, rotate
// what is due. It returns how many scheduled rotations it started.
func (e *Engine) Sweep(ctx context.Context) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	policies, err := e.Store.ListSecretRotationPolicies(ctx, "")
	if err != nil {
		e.log().Error("secret rotation: list policies", slog.String("error", err.Error()))
		return 0
	}
	limit := e.MaxPerPass
	if limit <= 0 {
		limit = DefaultMaxPerRun
	}
	now := e.now()
	rotated := 0
	for _, p := range policies {
		if ctx.Err() != nil {
			break
		}
		if e.dropIfAppGone(ctx, p) {
			continue
		}
		if p.State == store.SecretRotationRestartFailed || p.State == store.SecretRotationPending {
			_ = e.retryRestart(ctx, p, systemActor)
			continue
		}
		if !p.PreviousExpiresAt.IsZero() && !now.Before(p.PreviousExpiresAt) {
			e.expireGrace(ctx, p)
		}
		if p.State != store.SecretRotationIdle || !p.AutoRotate || rotated >= limit {
			continue
		}
		_, _, updated, kerr := e.Backend.KeyState(ctx, p.ServiceName, p.EnvKey)
		if kerr != nil {
			continue
		}
		due := NextDue(Basis(p.LastRotatedAt, updated), p.IntervalDays)
		if due.IsZero() || now.Before(due) {
			continue
		}
		rotated++
		if err := e.rotateLocked(ctx, p, systemActor); err != nil {
			e.log().Warn("secret rotation: scheduled rotation did not complete", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey))
		}
	}
	return rotated
}

func (e *Engine) dropIfAppGone(ctx context.Context, p store.SecretRotationPolicy) bool {
	exists, _, err := e.Backend.AppState(ctx, p.ServiceName)
	if err != nil || exists {
		return false
	}
	if _, err := e.Store.DeleteSecretRotationPolicy(ctx, p.ServiceName, p.EnvKey); err != nil {
		e.log().Warn("secret rotation: drop policy of deleted app", slog.String("app", p.ServiceName), slog.String("error", err.Error()))
	}
	return true
}

func (e *Engine) expireGrace(ctx context.Context, p store.SecretRotationPolicy) {
	if err := e.Backend.DeleteValue(ctx, p.ServiceName, PreviousKey(p.EnvKey)); err != nil {
		e.log().Warn("secret rotation: expire previous value", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
		return
	}
	p.PreviousExpiresAt = time.Time{}
	if err := e.Store.SaveSecretRotationProgress(ctx, p, e.now()); err != nil {
		e.log().Warn("secret rotation: record grace expiry", slog.String("app", p.ServiceName), slog.String("key", p.EnvKey), slog.String("error", err.Error()))
		return
	}
	e.audit(ctx, ActionGraceExpired, p.ServiceName, p.EnvKey, systemActor)
}

// Run sweeps at boot and then every interval until ctx ends.
func (e *Engine) Run(ctx context.Context, interval time.Duration) error {
	loop := footprint.Track("secret rotation sweep", interval)
	defer loop.Active()()
	sweep := func() {
		start := time.Now()
		e.Sweep(ctx)
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

package dbupgrade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment knobs. Every threshold is configurable, none is hardcoded.
const (
	EnvInterval           = "APP_DB_UPGRADE_INTERVAL"
	EnvHealthTimeout      = "APP_DB_UPGRADE_HEALTH_TIMEOUT"
	EnvMinWindowRemaining = "APP_DB_UPGRADE_MIN_WINDOW_REMAINING"
	EnvEOLWarn            = "APP_DB_EOL_WARN"
)

// Defaults for the knobs above.
const (
	DefaultInterval           = time.Minute
	DefaultMinWindowRemaining = 30 * time.Minute
	DefaultEOLWarn            = 180 * 24 * time.Hour
)

// DurationFromEnv parses key as a Go duration, falling back to def.
func DurationFromEnv(key string, def time.Duration, logger *slog.Logger) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logger.Warn("invalid duration in environment, using the default", slog.String("key", key), slog.String("value", raw))
		return def
	}
	return d
}

// Manager is the level-triggered controller: every tick it makes sure each
// unfinished run has a driver and starts runs that are due. It also backs
// the HTTP API (service.go).
type Manager struct {
	Store   Store
	Runner  *Runner
	Advisor Advisor
	Logger  *slog.Logger
	Now     func() time.Time
	// MinWindowRemaining stops a run starting too close to a window's end.
	MinWindowRemaining time.Duration
	// Unreachable returns why the runner cannot act on a database (for
	// example it runs on another node), or "". nil means always reachable.
	Unreachable func(db store.DesiredDatabase) string

	mu       sync.Mutex
	inflight map[string]bool
	root     context.Context
}

// Run ticks on interval until ctx is done; drivers inherit ctx.
func (m *Manager) Run(ctx context.Context, interval time.Duration) {
	m.mu.Lock()
	m.root = ctx
	m.mu.Unlock()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := m.Tick(ctx); err != nil {
			m.log().Warn("dbupgrade: tick had errors", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick resumes unfinished runs, then schedules due automatic upgrades.
func (m *Manager) Tick(ctx context.Context) error {
	active, err := m.Store.ListActiveDBUpgradeRuns(ctx)
	if err != nil {
		return fmt.Errorf("dbupgrade: list active runs: %w", err)
	}
	busy := map[string]bool{}
	for _, run := range active {
		busy[run.DatabaseName] = true
		m.startDriver(run.ID)
	}
	dbs, err := m.Store.ListDesiredDatabases(ctx)
	if err != nil {
		return fmt.Errorf("dbupgrade: list databases: %w", err)
	}
	var errs []error
	for _, db := range dbs {
		if busy[db.Name] {
			continue
		}
		if err := m.scheduleOne(ctx, db); err != nil {
			errs = append(errs, fmt.Errorf("database %q: %w", db.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) scheduleOne(ctx context.Context, db store.DesiredDatabase) error {
	if db.Suspended || db.BackupTargetID == "" || m.unreachable(db) != "" {
		return nil
	}
	policy, _, err := m.EffectivePolicy(ctx, db.Name)
	if err != nil {
		return err
	}
	now := m.now()
	open, err := policy.CanStartAt(now, m.minWindowRemaining())
	if err != nil || !open {
		return err
	}
	advice := m.Advisor.Advise(db.Engine, db.Version, now)
	target, ok := advice.BestAutomatic(policy.AutoUpgrade)
	if !ok {
		return nil
	}
	parked, err := m.parked(ctx, db.Name, target.Version)
	if err != nil || parked {
		return err
	}
	_, err = m.createRun(ctx, db, target, store.DBUpgradeSourceAuto, "", policy)
	return err
}

// parked is true when an automatic run to this exact version already failed
// or was reverted: it is not retried every window, an operator decides.
func (m *Manager) parked(ctx context.Context, name, version string) (bool, error) {
	runs, err := m.Store.ListDBUpgradeRuns(ctx, name, 0)
	if err != nil {
		return false, fmt.Errorf("list runs: %w", err)
	}
	for _, r := range runs {
		if r.ToVersion == version && (r.State == store.DBUpgradeStateFailed || r.State == store.DBUpgradeStateReverted) {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) createRun(ctx context.Context, db store.DesiredDatabase, target Target, source, by string, p Policy) (store.DBUpgradeRun, error) {
	now := m.now().UTC().Format(time.RFC3339)
	run := store.DBUpgradeRun{
		ID: newID("dbu_"), DatabaseName: db.Name, Engine: db.Engine, FromVersion: db.Version, ToVersion: target.Version,
		Kind: target.Kind, Source: source, State: store.DBUpgradeStatePending, VerifyAfter: p.VerifyAfter,
		RevertOnFailure: p.RevertOnFailure, Notify: p.Notify, RequestedBy: by,
		Timings: map[string]string{store.DBUpgradeStatePending: now}, CreatedAt: now, PhaseStartedAt: now,
	}
	if err := m.Store.CreateDBUpgradeRun(ctx, run); err != nil {
		return store.DBUpgradeRun{}, fmt.Errorf("create run: %w", err)
	}
	m.log().Info("dbupgrade: run created", slog.String("run", run.ID), slog.String("database", db.Name),
		slog.String("from", run.FromVersion), slog.String("to", run.ToVersion), slog.String("source", source))
	m.startDriver(run.ID)
	return run, nil
}

// startDriver runs the state machine for id in the background unless this
// process already drives it.
func (m *Manager) startDriver(id string) {
	m.mu.Lock()
	if m.inflight == nil {
		m.inflight = map[string]bool{}
	}
	if m.inflight[id] || m.root == nil {
		m.mu.Unlock()
		return
	}
	m.inflight[id] = true
	ctx := m.root
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.inflight, id)
			m.mu.Unlock()
		}()
		if err := m.Runner.Drive(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
			m.log().Error("dbupgrade: driver stopped", slog.String("run", id), slog.String("error", err.Error()))
		}
	}()
}

// EffectivePolicy returns the database's own policy, or the platform default
// with inherited=true.
func (m *Manager) EffectivePolicy(ctx context.Context, name string) (Policy, bool, error) {
	if name != store.DBUpgradePlatformDefault {
		own, found, err := m.Store.GetDBUpgradePolicy(ctx, name)
		if err != nil {
			return Policy{}, false, fmt.Errorf("load policy: %w", err)
		}
		if found {
			return PolicyFromStore(own), false, nil
		}
	}
	def, found, err := m.Store.GetDBUpgradePolicy(ctx, store.DBUpgradePlatformDefault)
	if err != nil {
		return Policy{}, false, fmt.Errorf("load platform default policy: %w", err)
	}
	if !found {
		return DefaultPolicy(), true, nil
	}
	return PolicyFromStore(def), name != store.DBUpgradePlatformDefault, nil
}

func (m *Manager) unreachable(db store.DesiredDatabase) string {
	if m.Unreachable == nil {
		return ""
	}
	return m.Unreachable(db)
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) minWindowRemaining() time.Duration {
	if m.MinWindowRemaining > 0 {
		return m.MinWindowRemaining
	}
	return DefaultMinWindowRemaining
}

func (m *Manager) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

// Package nodemove runs persisted, resumable moves of apps and databases
// between nodes, and node drains built on those moves. Every phase is
// idempotent and recorded before the next starts, so a restart resumes.
package nodemove

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Phases of a move, in order. Stateless moves skip stop_source and
// copy_volumes, stateful ones skip stop_old (the source is already stopped).
const (
	PhasePreflight   = "preflight"
	PhaseStopSource  = "stop_source"
	PhaseCopyVolumes = "copy_volumes"
	PhasePlace       = "place"
	PhaseVerify      = "verify"
	PhaseStopOld     = "stop_old"
	PhaseFinalize    = "finalize"
	PhaseRollback    = "rollback"
	PhaseDone        = "done"
)

// Step outcomes recorded in a move's history.
const (
	OutcomeOK      = "ok"
	OutcomeRetry   = "retry"
	OutcomeFailed  = "failed"
	OutcomeWaiting = "waiting"
)

// Store is the persistence the runner needs; *store.DB satisfies it.
type Store interface {
	CreateNodeMove(ctx context.Context, m store.NodeMove) error
	SaveNodeMove(ctx context.Context, m store.NodeMove) error
	GetNodeMove(ctx context.Context, id string) (store.NodeMove, error)
	ListNodeMoves(ctx context.Context, f store.NodeMoveFilter) ([]store.NodeMove, error)
	CreateNodeDrain(ctx context.Context, d store.NodeDrain) error
	SaveNodeDrain(ctx context.Context, d store.NodeDrain) error
	GetNodeDrain(ctx context.Context, id string) (store.NodeDrain, error)
	ListNodeDrains(ctx context.Context, nodeID string, limit int) ([]store.NodeDrain, error)
}

// Executor performs the side effects of each phase against the platform.
// Every method must be idempotent: a crash between an action and the
// phase save replays the action.
type Executor interface {
	// Preflight refuses a move that cannot work (wrap with Permanent) and
	// prepares dependencies such as a database's mesh endpoint.
	Preflight(ctx context.Context, m store.NodeMove) error
	// Stop suspends the resource and removes its containers from nodeID.
	Stop(ctx context.Context, kind, name, nodeID string) error
	CopyVolume(ctx context.Context, fromNodeID, toNodeID, volume string) error
	Place(ctx context.Context, kind, name, nodeID string) error
	Resume(ctx context.Context, kind, name string) error
	// Ready reports whether the resource runs and passes readiness on
	// nodeID, judged only on observations made after since.
	Ready(ctx context.Context, kind, name, nodeID string, since time.Time) (bool, string, error)
	// RemoveFrom removes the resource's containers from nodeID only.
	RemoveFrom(ctx context.Context, kind, name, nodeID string) error
	// Finalize runs after a verified move, e.g. restarting a database's dependents.
	Finalize(ctx context.Context, m store.NodeMove) error
}

// Config tunes the runner; ConfigFromEnv reads it from APP_NODE_MOVE_*.
type Config struct {
	VerifyTimeout time.Duration
	MaxAttempts   int
	Interval      time.Duration
}

// ConfigFromEnv reads APP_NODE_MOVE_VERIFY_TIMEOUT, APP_NODE_MOVE_MAX_ATTEMPTS
// and APP_NODE_MOVE_INTERVAL, falling back to defaults on unset or invalid values.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	cfg := Config{VerifyTimeout: 5 * time.Minute, MaxAttempts: 5, Interval: 3 * time.Second}
	if v, ok := lookup("APP_NODE_MOVE_VERIFY_TIMEOUT"); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.VerifyTimeout = d
		}
	}
	if v, ok := lookup("APP_NODE_MOVE_MAX_ATTEMPTS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxAttempts = n
		}
	}
	if v, ok := lookup("APP_NODE_MOVE_INTERVAL"); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Interval = d
		}
	}
	return cfg
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as not worth retrying.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// Runner advances moves and drains.
type Runner struct {
	store  Store
	exec   Executor
	drains DrainPlanner
	cfg    Config
	logger *slog.Logger
	now    func() time.Time
	newID  func(prefix string) (string, error)
	kick   chan struct{}
}

// Option configures a Runner.
type Option func(*Runner)

// WithClock overrides time.Now, for tests.
func WithClock(now func() time.Time) Option { return func(r *Runner) { r.now = now } }

// WithIDs overrides ID generation, for tests.
func WithIDs(f func(prefix string) (string, error)) Option { return func(r *Runner) { r.newID = f } }

// WithDrainPlanner enables drains.
func WithDrainPlanner(p DrainPlanner) Option { return func(r *Runner) { r.drains = p } }

// NewRunner builds a Runner.
func NewRunner(s Store, exec Executor, cfg Config, logger *slog.Logger, opts ...Option) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Runner{store: s, exec: exec, cfg: cfg, logger: logger, now: time.Now, newID: randomID, kick: make(chan struct{}, 1)}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Request starts one move.
type Request struct {
	Kind       string
	Resource   string
	FromNodeID string
	ToNodeID   string
	Volumes    []string
	DrainID    string
}

// Start records a new move and wakes the runner. It fails with
// store.ErrNodeMoveInProgress when the resource is already moving.
func (r *Runner) Start(ctx context.Context, req Request) (store.NodeMove, error) {
	if req.FromNodeID == req.ToNodeID {
		return store.NodeMove{}, Permanent(fmt.Errorf("%s %q is already on that node", req.Kind, req.Resource))
	}
	id, err := r.newID("mv")
	if err != nil {
		return store.NodeMove{}, err
	}
	now := r.stamp()
	m := store.NodeMove{
		ID: id, Kind: req.Kind, Resource: req.Resource, FromNodeID: req.FromNodeID, ToNodeID: req.ToNodeID,
		Stateful: req.Kind == store.NodeMoveKindDatabase || len(req.Volumes) > 0, Volumes: req.Volumes,
		Phase: PhasePreflight, Status: store.NodeMoveStatusRunning, DrainID: req.DrainID, StartedAt: now, UpdatedAt: now,
	}
	if err := r.store.CreateNodeMove(ctx, m); err != nil {
		return store.NodeMove{}, err
	}
	r.Kick()
	return m, nil
}

// Kick wakes Run without waiting for the next interval.
func (r *Runner) Kick() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// Run ticks until ctx is done. Running moves found at startup resume.
func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()
	for {
		r.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.kick:
		}
	}
}

// Tick advances every running move, then every running drain.
func (r *Runner) Tick(ctx context.Context) {
	moves, err := r.store.ListNodeMoves(ctx, store.NodeMoveFilter{Status: store.NodeMoveStatusRunning})
	if err != nil {
		r.logger.Error("nodemove: list running moves failed", slog.String("error", err.Error()))
	}
	for _, m := range moves {
		if _, err := r.Advance(ctx, m.ID); err != nil {
			r.logger.Error("nodemove: advance failed", slog.String("move_id", m.ID), slog.String("error", err.Error()))
		}
	}
	if r.drains == nil {
		return
	}
	drains, err := r.store.ListNodeDrains(ctx, "", 0)
	if err != nil {
		r.logger.Error("nodemove: list running drains failed", slog.String("error", err.Error()))
		return
	}
	for _, d := range drains {
		if err := r.AdvanceDrain(ctx, d.ID); err != nil {
			r.logger.Error("nodemove: advance drain failed", slog.String("drain_id", d.ID), slog.String("node_id", d.NodeID), slog.String("error", err.Error()))
		}
	}
}

// maxPhasesPerAdvance bounds one Advance call so a tick cannot spin.
const maxPhasesPerAdvance = 10

// Advance runs phases of one move until it waits, finishes, or a phase
// needs a retry. It returns the persisted move.
func (r *Runner) Advance(ctx context.Context, id string) (store.NodeMove, error) {
	m, err := r.store.GetNodeMove(ctx, id)
	if err != nil {
		return m, fmt.Errorf("nodemove: load %s: %w", id, err)
	}
	for i := 0; i < maxPhasesPerAdvance && m.Status == store.NodeMoveStatusRunning; i++ {
		before := m.Phase
		progressed := r.step(ctx, &m)
		m.UpdatedAt = r.stamp()
		if err := r.store.SaveNodeMove(ctx, m); err != nil {
			return m, fmt.Errorf("nodemove: save %s after %s: %w", id, before, err)
		}
		if !progressed {
			break
		}
	}
	return m, nil
}

// step executes m's current phase once, mutating m. It reports whether
// the phase completed and the next one may run immediately.
func (r *Runner) step(ctx context.Context, m *store.NodeMove) bool {
	switch m.Phase {
	case PhasePreflight:
		return r.do(m, r.exec.Preflight(ctx, *m), r.afterPreflight(m))
	case PhaseStopSource:
		return r.do(m, r.exec.Stop(ctx, m.Kind, m.Resource, m.FromNodeID), PhaseCopyVolumes)
	case PhaseCopyVolumes:
		for _, v := range m.Volumes {
			if err := r.exec.CopyVolume(ctx, m.FromNodeID, m.ToNodeID, v); err != nil {
				return r.do(m, fmt.Errorf("copy volume %s: %w", v, err), PhasePlace)
			}
		}
		return r.do(m, nil, PhasePlace)
	case PhasePlace:
		return r.place(ctx, m)
	case PhaseVerify:
		return r.verify(ctx, m)
	case PhaseStopOld:
		return r.do(m, r.exec.RemoveFrom(ctx, m.Kind, m.Resource, m.FromNodeID), PhaseFinalize)
	case PhaseFinalize:
		if err := r.exec.Finalize(ctx, *m); err != nil {
			// The move itself is verified; a dependent restart failing must not undo it.
			r.record(m, PhaseFinalize, OutcomeFailed, err.Error())
			m.Error = "moved, but follow-up failed: " + err.Error()
		} else {
			r.record(m, PhaseFinalize, OutcomeOK, "")
		}
		r.finish(m, store.NodeMoveStatusSucceeded)
		return false
	case PhaseRollback:
		return r.rollback(ctx, m)
	default:
		r.fail(m, store.NodeMoveStatusFailed, "unknown phase "+m.Phase)
		return false
	}
}

func (r *Runner) afterPreflight(m *store.NodeMove) string {
	if m.Stateful {
		return PhaseStopSource
	}
	return PhasePlace
}

// do records the outcome of a phase action and moves to next on success.
// A failure retries until MaxAttempts, then rolls back (or fails outright
// in preflight, where nothing has changed yet).
func (r *Runner) do(m *store.NodeMove, err error, next string) bool {
	if err == nil {
		r.record(m, m.Phase, OutcomeOK, "")
		m.Phase, m.Attempts = next, 0
		return true
	}
	m.Attempts++
	if !IsPermanent(err) && m.Attempts < r.cfg.MaxAttempts {
		r.record(m, m.Phase, OutcomeRetry, err.Error())
		return false
	}
	r.record(m, m.Phase, OutcomeFailed, err.Error())
	if m.Phase == PhasePreflight {
		r.fail(m, store.NodeMoveStatusFailed, err.Error())
		return false
	}
	m.Error = fmt.Sprintf("%s failed: %s", m.Phase, err.Error())
	m.Phase, m.Attempts = PhaseRollback, 0
	return true
}

func (r *Runner) place(ctx context.Context, m *store.NodeMove) bool {
	if err := r.exec.Place(ctx, m.Kind, m.Resource, m.ToNodeID); err != nil {
		return r.do(m, err, PhaseVerify)
	}
	now := r.now().UTC()
	m.PlacedAt = now.Format(time.RFC3339Nano)
	if m.Stateful {
		if err := r.exec.Resume(ctx, m.Kind, m.Resource); err != nil {
			return r.do(m, fmt.Errorf("resume on target: %w", err), PhaseVerify)
		}
	}
	m.VerifyDeadline = now.Add(r.cfg.VerifyTimeout).Format(time.RFC3339Nano)
	return r.do(m, nil, PhaseVerify)
}

func (r *Runner) verify(ctx context.Context, m *store.NodeMove) bool {
	since, _ := time.Parse(time.RFC3339Nano, m.PlacedAt)
	ready, reason, err := r.exec.Ready(ctx, m.Kind, m.Resource, m.ToNodeID, since)
	if err != nil {
		return r.do(m, fmt.Errorf("check readiness: %w", err), r.afterVerify(m))
	}
	if ready {
		return r.do(m, nil, r.afterVerify(m))
	}
	deadline, _ := time.Parse(time.RFC3339Nano, m.VerifyDeadline)
	if !r.now().Before(deadline) {
		if reason == "" {
			reason = "no ready report from the target node"
		}
		return r.do(m, Permanent(fmt.Errorf("not ready on the target node within %s: %s", r.cfg.VerifyTimeout, reason)), r.afterVerify(m))
	}
	if last := lastStep(m); last == nil || last.Phase != PhaseVerify || last.Outcome != OutcomeWaiting || last.Message != reason {
		r.record(m, PhaseVerify, OutcomeWaiting, reason)
	}
	return false
}

func (r *Runner) afterVerify(m *store.NodeMove) string {
	if m.Stateful {
		return PhaseFinalize
	}
	return PhaseStopOld
}

// rollback returns the resource to its source node. Stateful sources were
// stopped but never modified (copies only read them), so resuming there
// restores the pre-move data.
func (r *Runner) rollback(ctx context.Context, m *store.NodeMove) bool {
	placed := m.PlacedAt != ""
	var err error
	switch {
	case m.Stateful && placed:
		if err = r.exec.Stop(ctx, m.Kind, m.Resource, m.ToNodeID); err == nil {
			if err = r.exec.Place(ctx, m.Kind, m.Resource, m.FromNodeID); err == nil {
				err = r.exec.Resume(ctx, m.Kind, m.Resource)
			}
		}
	case m.Stateful:
		err = r.exec.Resume(ctx, m.Kind, m.Resource)
	case placed:
		if err = r.exec.Place(ctx, m.Kind, m.Resource, m.FromNodeID); err == nil {
			err = r.exec.RemoveFrom(ctx, m.Kind, m.Resource, m.ToNodeID)
		}
	}
	if err == nil {
		r.record(m, PhaseRollback, OutcomeOK, "back on the source node")
		r.finish(m, store.NodeMoveStatusRolledBack)
		return false
	}
	m.Attempts++
	if m.Attempts < r.cfg.MaxAttempts {
		r.record(m, PhaseRollback, OutcomeRetry, err.Error())
		return false
	}
	r.record(m, PhaseRollback, OutcomeFailed, err.Error())
	r.fail(m, store.NodeMoveStatusFailed, m.Error+"; rollback also failed, check the resource by hand: "+err.Error())
	return false
}

func (r *Runner) fail(m *store.NodeMove, status, msg string) {
	m.Error = msg
	r.finish(m, status)
}

func (r *Runner) finish(m *store.NodeMove, status string) {
	m.Status = status
	m.Phase = PhaseDone
	m.FinishedAt = r.stamp()
}

func (r *Runner) record(m *store.NodeMove, phase, outcome, msg string) {
	m.Steps = append(m.Steps, store.NodeMoveStep{Phase: phase, Outcome: outcome, Message: msg, At: r.stamp()})
	if len(m.Steps) > maxSteps {
		m.Steps = m.Steps[len(m.Steps)-maxSteps:]
	}
}

// maxSteps caps history so a long verify wait cannot grow the row unbounded.
const maxSteps = 50

func lastStep(m *store.NodeMove) *store.NodeMoveStep {
	if len(m.Steps) == 0 {
		return nil
	}
	return &m.Steps[len(m.Steps)-1]
}

func (r *Runner) stamp() string { return r.now().UTC().Format(time.RFC3339Nano) }

// Package stacksync plans stacks from their opt-in git source. It never
// applies, prunes, deploys or writes back to git; it records a plan result.
package stacksync

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Event types the syncer emits.
const (
	EventPending = "stacks.sync_pending"
	EventFailed  = "stacks.sync_failed"
)

// Store is the slice of the store the syncer needs.
type Store interface {
	ListEnabledStackSyncs(ctx context.Context) ([]store.StackSyncRecord, error)
	SetStackSyncStatus(ctx context.Context, stackID string, st store.StackSyncStatus) error
}

// Fetch checks out the configured branch and returns its commit; cleanup removes the tree.
type Fetch func(ctx context.Context, rec store.StackSyncRecord) (dir, commit string, cleanup func(), err error)

// Plan diffs the stack's documents against live state; it must not write.
type Plan func(ctx context.Context, rec store.StackSyncRecord, files []File) (iac.Plan, error)

// Emit sends one event; status carries codes and counts only.
type Emit func(ctx context.Context, typ string, rec store.StackSyncRecord, st store.StackSyncStatus)

// Limits are read from runtime settings on every tick.
type Limits struct {
	Tick         time.Duration
	MinInterval  time.Duration
	FetchTimeout time.Duration
	MaxRepoBytes int64
	MaxFileBytes int64
	MaxFiles     int
}

// CodedError carries a stable code; its message never holds repository content.
type CodedError struct {
	Code string
	Msg  string
}

func (e *CodedError) Error() string { return e.Code + ": " + e.Msg }

// Syncer runs the loop only while at least one stack has sync enabled.
type Syncer struct {
	Store  Store
	Fetch  Fetch
	Plan   Plan
	Emit   Emit
	Limits func(ctx context.Context) Limits
	Logger *slog.Logger
	Now    func() time.Time

	mu      sync.Mutex
	base    context.Context
	running bool
	last    map[string]time.Time
	stackMu sync.Map
}

// Start records the lifetime context and starts the loop if any stack syncs.
func (s *Syncer) Start(ctx context.Context) {
	s.mu.Lock()
	s.base = ctx
	s.mu.Unlock()
	// No goroutine at all while idle; enabling a stack calls Kick.
	if recs, err := s.Store.ListEnabledStackSyncs(ctx); err == nil && len(recs) == 0 {
		return
	}
	s.Kick()
}

// Kick starts the loop when it is not running; a no-op before Start.
func (s *Syncer) Kick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base == nil || s.running || s.base.Err() != nil {
		return
	}
	s.running = true
	go s.loop(s.base)
}

// Running reports whether the loop goroutine is alive.
func (s *Syncer) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Syncer) loop(ctx context.Context) {
	for {
		if !s.pass(ctx) {
			return
		}
		select {
		case <-ctx.Done():
			s.stop()
			return
		case <-time.After(s.limits(ctx).Tick):
		}
	}
}

func (s *Syncer) stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// pass syncs every due stack and reports false (loop stopped) when none is enabled.
func (s *Syncer) pass(ctx context.Context) bool {
	recs, err := s.Store.ListEnabledStackSyncs(ctx)
	if err != nil {
		s.logger().Warn("stack sync: list failed", slog.String("error", err.Error()))
		return true
	}
	if len(recs) == 0 {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Re-check under the lock so a Kick racing this exit is not lost.
		if again, err := s.Store.ListEnabledStackSyncs(ctx); err == nil && len(again) > 0 {
			return true
		}
		s.running = false
		return false
	}
	lim := s.limits(ctx)
	for _, rec := range recs {
		if ctx.Err() != nil {
			return true
		}
		if rec.Config.WebhookOnly || !s.due(rec, lim) {
			continue
		}
		s.SyncOne(ctx, rec)
	}
	return true
}

func (s *Syncer) due(rec store.StackSyncRecord, lim Limits) bool {
	every := time.Duration(rec.Config.IntervalSeconds) * time.Second
	if every < lim.MinInterval {
		every = lim.MinInterval
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.last[rec.StackID]
	return !ok || s.now().Sub(last) >= every
}

// SyncOne fetches, reads and plans one stack, stores the status and emits
// pending once per new commit and failed once per new code.
func (s *Syncer) SyncOne(ctx context.Context, rec store.StackSyncRecord) store.StackSyncStatus {
	lockAny, _ := s.stackMu.LoadOrStore(rec.StackID, &sync.Mutex{})
	lock := lockAny.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	s.mu.Lock()
	if s.last == nil {
		s.last = map[string]time.Time{}
	}
	s.last[rec.StackID] = s.now()
	s.mu.Unlock()

	prev := rec.Status
	st := store.StackSyncStatus{LastSHA: prev.LastSHA, NotifiedSHA: prev.NotifiedSHA, CheckedAt: s.now().UTC()}
	plan, commit, err := s.run(ctx, rec)
	if commit != "" {
		st.LastSHA = commit
	}
	if err != nil {
		st.Code = codeOf(err)
		s.logger().Warn("stack sync failed", slog.String("stack", rec.StackName), slog.String("code", st.Code))
		if st.Code != prev.Code {
			s.emit(ctx, EventFailed, rec, st)
		}
	} else {
		st.PlanHash = plan.Hash
		st.Counts = map[string]int{"create": plan.Summary.Create, "update": plan.Summary.Update, "delete": plan.Summary.Delete}
		st.Pending = plan.Pending()
		if st.Pending && commit != prev.NotifiedSHA {
			st.NotifiedSHA = commit
			s.emit(ctx, EventPending, rec, st)
		}
	}
	if serr := s.Store.SetStackSyncStatus(context.WithoutCancel(ctx), rec.StackID, st); serr != nil {
		s.logger().Warn("stack sync: save status failed", slog.String("stack", rec.StackName), slog.String("error", serr.Error()))
	}
	return st
}

func (s *Syncer) run(ctx context.Context, rec store.StackSyncRecord) (iac.Plan, string, error) {
	lim := s.limits(ctx)
	cfg := iac.StackSync{RepoURL: rec.Config.RepoURL, Branch: rec.Config.Branch, Path: rec.Config.Path, CredentialRef: rec.Config.CredentialRef}
	if code, err := cfg.Validate(); err != nil {
		return iac.Plan{}, "", &CodedError{Code: code, Msg: "sync settings no longer validate"}
	}
	fctx, cancel := context.WithTimeout(ctx, lim.FetchTimeout)
	dir, commit, cleanup, err := s.Fetch(fctx, rec)
	cancel()
	if err != nil {
		var ce *CodedError
		if errors.As(err, &ce) {
			return iac.Plan{}, "", ce
		}
		return iac.Plan{}, "", &CodedError{Code: iac.CodeSyncRepoUnreachable, Msg: "fetch failed"}
	}
	if cleanup != nil {
		defer cleanup()
	}
	files, err := ReadTree(dir, rec.Config.Path, lim)
	if err != nil {
		return iac.Plan{}, commit, err
	}
	plan, err := s.Plan(ctx, rec, files)
	if err != nil {
		var ce *CodedError
		if errors.As(err, &ce) {
			return iac.Plan{}, commit, ce
		}
		return iac.Plan{}, commit, &CodedError{Code: iac.CodeSyncPlanFailed, Msg: "plan failed"}
	}
	return plan, commit, nil
}

func codeOf(err error) string {
	var ce *CodedError
	if errors.As(err, &ce) && ce.Code != "" {
		return ce.Code
	}
	return iac.CodeSyncPlanFailed
}

func (s *Syncer) emit(ctx context.Context, typ string, rec store.StackSyncRecord, st store.StackSyncStatus) {
	if s.Emit != nil {
		s.Emit(ctx, typ, rec, st)
	}
}

// DefaultLimits mirror the registered setting defaults for callers without settings.
func DefaultLimits() Limits {
	return Limits{Tick: time.Minute, MinInterval: time.Minute, FetchTimeout: 2 * time.Minute, MaxRepoBytes: 50 << 20, MaxFileBytes: 1 << 20, MaxFiles: 200}
}

func (s *Syncer) limits(ctx context.Context) Limits {
	d := DefaultLimits()
	if s.Limits == nil {
		return d
	}
	l := s.Limits(ctx)
	if l.Tick <= 0 {
		l.Tick = d.Tick
	}
	if l.MinInterval <= 0 {
		l.MinInterval = d.MinInterval
	}
	if l.FetchTimeout <= 0 {
		l.FetchTimeout = d.FetchTimeout
	}
	if l.MaxRepoBytes <= 0 {
		l.MaxRepoBytes = d.MaxRepoBytes
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = d.MaxFileBytes
	}
	if l.MaxFiles <= 0 {
		l.MaxFiles = d.MaxFiles
	}
	return l
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Syncer) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

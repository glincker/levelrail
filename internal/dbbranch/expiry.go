package dbbranch

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// ExpiryStore is the store surface of the expiry job.
type ExpiryStore interface {
	Store
	ListAllDatabaseBranches(ctx context.Context) ([]store.DatabaseBranch, error)
}

// ExpiryPolicy is read from the live branches.* settings on every tick.
type ExpiryPolicy struct {
	Enabled  bool
	Interval time.Duration
	// Warning is how long before expiry a warning goes out; zero disables it.
	Warning         time.Duration
	FailedRetention time.Duration
	// Retry is how long an expiring or deleting row may sit before the job resumes it.
	Retry time.Duration
	// RequestedTimeout is how long a requested row may wait for its clone to start.
	RequestedTimeout time.Duration
}

// ExpiryJob deletes due branches through the normal teardown and warns before expiry.
// Protected rows and rows with live children are never selected (store.SelectDueBranches).
type ExpiryJob struct {
	Store  ExpiryStore
	Policy func(ctx context.Context) ExpiryPolicy
	// Delete runs the normal teardown and removes the row; it must honor the store guards.
	Delete func(ctx context.Context, b store.DatabaseBranch) error
	// Warn is the existing notification path; optional.
	Warn func(ctx context.Context, b store.DatabaseBranch, expiresAt time.Time)
	Now  func() time.Time
	Log  *slog.Logger

	mu     sync.Mutex
	warned map[string]string
}

func (j *ExpiryJob) now() time.Time {
	if j.Now != nil {
		return j.Now()
	}
	return time.Now()
}

// Run ticks until ctx ends. The interval is re-read each round so a settings change applies live.
func (j *ExpiryJob) Run(ctx context.Context) {
	for {
		p := j.Policy(ctx)
		wait := p.Interval
		if wait <= 0 {
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		j.Tick(ctx)
	}
}

// Tick runs one collection and warning pass.
func (j *ExpiryJob) Tick(ctx context.Context) {
	p := j.Policy(ctx)
	if !p.Enabled {
		return
	}
	rows, err := j.Store.ListAllDatabaseBranches(ctx)
	if err != nil {
		j.warn("list branches", "", err)
		return
	}
	now := j.now()
	j.warnUpcoming(ctx, rows, now, p)
	due := SelectExpired(rows, now, store.DuePolicy{FailedRetention: p.FailedRetention, Retry: p.Retry, RequestedTimeout: p.RequestedTimeout})
	for _, b := range due {
		if err := j.collect(ctx, b, now); err != nil {
			j.warn("collect branch", b.DatabaseName, err)
		}
	}
}

// collect moves a due row to expiring (through failed for a stalled request) and deletes it.
func (j *ExpiryJob) collect(ctx context.Context, b store.DatabaseBranch, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	cur := b.State
	if cur == StateRequested {
		if err := Move(ctx, j.Store, b.ID, cur, StateFailed, "clone never started", ts); err != nil {
			return err
		}
		cur = StateFailed
	}
	if cur == StateReady || cur == StateFailed {
		if err := Move(ctx, j.Store, b.ID, cur, StateExpiring, "expired", ts); err != nil {
			return err
		}
	}
	return j.Delete(ctx, b)
}

func (j *ExpiryJob) warnUpcoming(ctx context.Context, rows []store.DatabaseBranch, now time.Time, p ExpiryPolicy) {
	if j.Warn == nil || p.Warning <= 0 {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.warned == nil {
		j.warned = map[string]string{}
	}
	for _, b := range rows {
		if b.State != StateReady || b.Protected || b.ExpiresAt == "" || j.warned[b.ID] == b.ExpiresAt {
			continue
		}
		t, err := time.Parse(time.RFC3339, b.ExpiresAt)
		if err != nil || t.Before(now) || t.Sub(now) > p.Warning {
			continue
		}
		j.warned[b.ID] = b.ExpiresAt
		j.Warn(ctx, b, t)
	}
}

func (j *ExpiryJob) warn(msg, name string, err error) {
	if j.Log != nil {
		j.Log.Warn("dbbranch expiry: "+msg, slog.String("branch", name), slog.String("error", err.Error()))
	}
}

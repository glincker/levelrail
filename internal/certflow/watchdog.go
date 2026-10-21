package certflow

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Env names for the renewal watchdog.
const (
	EnvWatchdogInterval = "APP_CERT_WATCHDOG_INTERVAL"
	EnvWatchdogBackoff  = "APP_CERT_WATCHDOG_BACKOFF"

	defaultWatchdogInterval = 10 * time.Minute
	defaultWatchdogBackoff  = 30 * time.Minute
	maxWatchdogBackoff      = 12 * time.Hour
)

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

// Candidate is a certificate whose renewal failed or is stuck.
type Candidate struct {
	Domain string
	Reason string
	// Expired certificates are cleared before re-issuing; valid ones are left in place.
	Expired bool
	// Blocked is set while a CA rate limit makes a try pointless.
	Blocked bool
}

// WatchdogDeps are the seams the watchdog drives; it has no ACME code.
type WatchdogDeps struct {
	Scan  func(ctx context.Context) ([]Candidate, error)
	Renew func(ctx context.Context, c Candidate) error
	Now   func() time.Time
	Log   *slog.Logger
}

// RetryState is one stuck domain's retry bookkeeping.
type RetryState struct {
	Attempts int
	NextAt   time.Time
	LastErr  string
}

// Watchdog retries stuck renewals with exponential backoff per domain.
type Watchdog struct {
	deps     WatchdogDeps
	interval time.Duration
	base     time.Duration
	mu       sync.Mutex
	state    map[string]*RetryState
}

// NewWatchdog builds a watchdog, reading its timings from the environment.
func NewWatchdog(deps WatchdogDeps) *Watchdog {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Watchdog{
		deps:     deps,
		interval: envDuration(EnvWatchdogInterval, defaultWatchdogInterval),
		base:     envDuration(EnvWatchdogBackoff, defaultWatchdogBackoff),
		state:    map[string]*RetryState{},
	}
}

func backoff(base time.Duration, attempts int) time.Duration {
	d := base
	for i := 1; i < attempts && d < maxWatchdogBackoff; i++ {
		d *= 2
	}
	return min(d, maxWatchdogBackoff)
}

// State returns the retry bookkeeping for domain, if it is being retried.
func (w *Watchdog) State(domain string) (RetryState, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.state[domain]
	if !ok {
		return RetryState{}, false
	}
	return *s, true
}

// Tick runs one pass: it retries every due candidate and forgets domains that recovered.
func (w *Watchdog) Tick(ctx context.Context) {
	cands, err := w.deps.Scan(ctx)
	if err != nil {
		w.deps.Log.Warn("certflow: watchdog scan failed", slog.String("error", err.Error()))
		return
	}
	now := w.deps.Now()
	live := map[string]bool{}
	for _, c := range cands {
		live[c.Domain] = true
		w.retry(ctx, c, now)
	}
	w.mu.Lock()
	for d := range w.state {
		if !live[d] {
			delete(w.state, d)
		}
	}
	w.mu.Unlock()
}

func (w *Watchdog) retry(ctx context.Context, c Candidate, now time.Time) {
	w.mu.Lock()
	s, ok := w.state[c.Domain]
	if !ok {
		s = &RetryState{}
		w.state[c.Domain] = s
	}
	due := !now.Before(s.NextAt)
	w.mu.Unlock()
	if !due || c.Blocked {
		return
	}
	err := w.deps.Renew(ctx, c)
	w.mu.Lock()
	defer w.mu.Unlock()
	s.Attempts++
	s.NextAt = now.Add(backoff(w.base, s.Attempts))
	s.LastErr = ""
	if err != nil {
		s.LastErr = err.Error()
	}
	w.deps.Log.Info("certflow: watchdog retried renewal", slog.String("domain", c.Domain),
		slog.String("reason", c.Reason), slog.Int("attempt", s.Attempts), slog.Time("next_at", s.NextAt))
}

// Run ticks until ctx ends.
func (w *Watchdog) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.Tick(ctx)
		}
	}
}

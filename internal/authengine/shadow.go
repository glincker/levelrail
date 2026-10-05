package authengine

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Authenticator is the library side of a shadow comparison.
type Authenticator interface {
	Authenticate(ctx context.Context, raw string) (Decision, error)
}

// LegacyOutcome is what the legacy engine decided for a bearer token.
// An accepted token with no OwnerID is a system token the library never holds, so it is skipped.
type LegacyOutcome struct {
	Accepted  bool
	TokenID   string
	OwnerID   string
	Abilities []string
}

// LegacyFunc evaluates a secret with the legacy engine. It runs on a shadow
// worker, so the request path does no extra work.
type LegacyFunc func(ctx context.Context, raw string) (LegacyOutcome, error)

const compareTimeout = 5 * time.Second

// Shadow compares the library's bearer decision with the legacy one off the
// request path. Observe never blocks: a full queue drops and counts.
type Shadow struct {
	auth   Authenticator
	legacy LegacyFunc
	logger *slog.Logger
	queue  chan string
	wg     sync.WaitGroup
	closed atomic.Bool

	compared, matched, mismatched, dropped, skipped, errored atomic.Uint64

	mu     sync.Mutex
	recent []Mismatch
	limit  int
}

// NewShadow starts the comparison workers.
func NewShadow(a Authenticator, legacy LegacyFunc, cfg ShadowConfig, logger *slog.Logger) *Shadow {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultShadowQueue
	}
	if cfg.Workers <= 0 {
		cfg.Workers = defaultShadowWorkers
	}
	if cfg.MismatchLog <= 0 {
		cfg.MismatchLog = defaultShadowMismatchLog
	}
	s := &Shadow{auth: a, legacy: legacy, logger: logger, queue: make(chan string, cfg.QueueSize), limit: cfg.MismatchLog}
	for range cfg.Workers {
		s.wg.Add(1)
		go s.run()
	}
	return s
}

// Observe queues a comparison. It reports false when the observation was dropped.
func (s *Shadow) Observe(raw string) bool {
	if s.closed.Load() {
		s.dropped.Add(1)
		return false
	}
	select {
	case s.queue <- raw:
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

// Close stops the workers after they drain what is queued.
func (s *Shadow) Close() {
	if s.closed.CompareAndSwap(false, true) {
		close(s.queue)
	}
	s.wg.Wait()
}

func (s *Shadow) run() {
	defer s.wg.Done()
	for raw := range s.queue {
		s.compare(raw)
	}
}

func normalizeAbilities(in []string) []string {
	if slices.Contains(in, AbilityRoot) {
		return []string{AbilityRoot}
	}
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func (s *Shadow) compare(raw string) {
	ctx, cancel := context.WithTimeout(context.Background(), compareTimeout)
	defer cancel()
	legacy, err := s.legacy(ctx, raw)
	if err != nil {
		s.errored.Add(1)
		s.logger.Warn("auth shadow: legacy evaluation failed", slog.String("error", err.Error()))
		return
	}
	lib, err := s.auth.Authenticate(ctx, raw)
	if err != nil {
		s.errored.Add(1)
		s.logger.Warn("auth shadow: library authenticate failed", slog.String("token_id", legacy.TokenID), slog.String("error", err.Error()))
		return
	}
	s.judge(legacy, lib)
}

func (s *Shadow) judge(legacy LegacyOutcome, lib Decision) {
	if legacy.Accepted && legacy.OwnerID == "" && !lib.Accepted {
		s.skipped.Add(1)
		return
	}
	s.compared.Add(1)
	kind := ""
	switch {
	case legacy.Accepted != lib.Accepted:
		kind = MismatchDecision
	case !lib.Accepted:
	case legacy.OwnerID != lib.OwnerID:
		kind = MismatchOwner
	case !slices.Equal(normalizeAbilities(legacy.Abilities), normalizeAbilities(lib.Abilities)):
		kind = MismatchAbilities
	}
	if kind == "" {
		s.matched.Add(1)
		return
	}
	s.mismatched.Add(1)
	m := Mismatch{
		At: time.Now().UTC(), Kind: kind, TokenID: legacy.TokenID,
		LegacyOwnerID: legacy.OwnerID, LibraryOwnerID: lib.OwnerID,
		LegacyAccepted: legacy.Accepted, LibraryAccepted: lib.Accepted,
		LegacyAbilities: normalizeAbilities(legacy.Abilities), LibraryAbilities: normalizeAbilities(lib.Abilities),
	}
	s.record(m)
	s.logger.Warn("auth shadow: mismatch", slog.String("kind", kind), slog.String("token_id", m.TokenID),
		slog.String("legacy_owner_id", m.LegacyOwnerID), slog.String("library_owner_id", m.LibraryOwnerID))
}

func (s *Shadow) record(m Mismatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recent = append(s.recent, m)
	if len(s.recent) > s.limit {
		s.recent = s.recent[len(s.recent)-s.limit:]
	}
}

// Snapshot returns the counters and the recent mismatches, newest first.
func (s *Shadow) Snapshot() Status {
	s.mu.Lock()
	recent := make([]Mismatch, len(s.recent))
	for i, m := range s.recent {
		recent[len(s.recent)-1-i] = m
	}
	s.mu.Unlock()
	return Status{
		Compared: s.compared.Load(), Matched: s.matched.Load(), Mismatched: s.mismatched.Load(),
		Dropped: s.dropped.Load(), Skipped: s.skipped.Load(), Errors: s.errored.Load(), Mismatches: recent,
	}
}

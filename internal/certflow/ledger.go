package certflow

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// Env names for the Let's Encrypt limits the ledger reasons about.
const (
	EnvFailedPerHour    = "APP_ACME_FAILED_PER_HOUR"
	EnvDuplicatePerWeek = "APP_ACME_DUPLICATE_PER_WEEK"

	defaultFailedPerHour    = 5
	defaultDuplicatePerWeek = 5
	failedWindow            = time.Hour
	duplicateWindow         = 7 * 24 * time.Hour
	rateLimitedPause        = time.Hour
	// ledgerKeep bounds memory: older entries cannot affect either window.
	ledgerKeep = duplicateWindow
	// LedgerKeep is how long stored attempts matter.
	LedgerKeep = ledgerKeep
)

// Limits are the CA limits that matter for one hostname.
type Limits struct {
	FailedPerHour    int
	DuplicatePerWeek int
}

// LimitsFromEnv reads the limits, defaulting to Let's Encrypt's published ones.
func LimitsFromEnv() Limits {
	return Limits{
		FailedPerHour:    envPositive(EnvFailedPerHour, defaultFailedPerHour),
		DuplicatePerWeek: envPositive(EnvDuplicatePerWeek, defaultDuplicatePerWeek),
	}
}

func envPositive(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

// Ledger remembers recent production attempts per hostname so the next safe
// time can be shown before another try burns a limit. Attach a Backing to keep
// it across restarts; without one it lives in memory and the CA's answer wins.
type Ledger struct {
	mu     sync.Mutex
	back   Backing
	failed map[string][]time.Time
	issued map[string][]time.Time
	start  map[string]time.Time
}

// Entry kinds written to a Backing.
const (
	KindFailed  = "failed"
	KindIssued  = "issued"
	KindStarted = "started"
)

// Backing persists ledger entries. Save is best effort: a failed write must not block issuance.
type Backing interface {
	Save(kind, domain string, at time.Time)
}

// Restore replays stored entries into the ledger without writing them back, then
// sends later changes to b.
func (l *Ledger) Restore(b Backing, entries []Entry, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range entries {
		switch e.Kind {
		case KindFailed:
			l.add(&l.failed, e.Domain, e.At, now)
		case KindIssued:
			l.add(&l.issued, e.Domain, e.At, now)
		case KindStarted:
			if l.start == nil {
				l.start = map[string]time.Time{}
			}
			if e.At.After(l.start[e.Domain]) {
				l.start[e.Domain] = e.At
			}
		}
	}
	l.back = b
}

// Entry is one stored attempt.
type Entry struct {
	Kind, Domain string
	At           time.Time
}

func (l *Ledger) save(kind, domain string, at time.Time) {
	if l.back != nil {
		l.back.Save(kind, domain, at)
	}
}

// MarkStarted notes when a guided issuance began for domain.
func (l *Ledger) MarkStarted(domain string, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.start == nil {
		l.start = map[string]time.Time{}
	}
	l.start[domain] = at
	l.save(KindStarted, domain, at)
}

// StartedAt is when the last guided issuance for domain began, or the zero time.
func (l *Ledger) StartedAt(domain string) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.start[domain]
}

func (l *Ledger) add(m *map[string][]time.Time, domain string, at, now time.Time) {
	if *m == nil {
		*m = map[string][]time.Time{}
	}
	for _, t := range (*m)[domain] {
		if t.Equal(at) {
			return
		}
	}
	kept := (*m)[domain][:0]
	for _, t := range (*m)[domain] {
		if now.Sub(t) < ledgerKeep {
			kept = append(kept, t)
		}
	}
	(*m)[domain] = append(kept, at)
}

// RecordFailure notes a failed production validation; repeats of one timestamp are ignored.
func (l *Ledger) RecordFailure(domain string, at, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.add(&l.failed, domain, at, now)
	l.save(KindFailed, domain, at)
}

// RecordIssued notes a production certificate issued for domain.
func (l *Ledger) RecordIssued(domain string, at, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.add(&l.issued, domain, at, now)
	l.save(KindIssued, domain, at)
}

// Snapshot is the recent history of one hostname.
type Snapshot struct {
	Failed []time.Time
	Issued []time.Time
}

// Snapshot returns a copy of domain's recent history.
func (l *Ledger) Snapshot(domain string) Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Snapshot{
		Failed: append([]time.Time(nil), l.failed[domain]...),
		Issued: append([]time.Time(nil), l.issued[domain]...),
	}
}

// IssuedSince counts production certificates issued in the last window across
// every hostname, which is what a shared weekly budget is spent on.
func (l *Ledger) IssuedSince(now time.Time, window time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, ts := range l.issued {
		for _, t := range ts {
			if now.Sub(t) < window {
				n++
			}
		}
	}
	return n
}

// BudgetWindow is the span the CA's duplicate and new-order limits are counted over.
const BudgetWindow = duplicateWindow

// RateLimit is the verdict on trying again now.
type RateLimit struct {
	Blocked bool
	// NextSafeAt is the earliest time another production try is safe.
	NextSafeAt time.Time
	// Reason is "failed_validations", "duplicate_certificates" or "ca_rate_limited".
	Reason string
}

// Rate limit reasons.
const (
	ReasonFailedValidations = "failed_validations"
	ReasonDuplicates        = "duplicate_certificates"
	ReasonCARateLimited     = "ca_rate_limited"
)

// Check decides whether a production attempt is safe at now. lastCAErr is the
// CA's latest error text and lastCAErrAt its time, so a "rate limited" answer
// pauses even when this process never counted the attempts.
func (s Snapshot) Check(lim Limits, now time.Time, lastCAErr string, lastCAErrAt time.Time) RateLimit {
	if oldest, n := within(s.Failed, now, failedWindow); n >= lim.FailedPerHour {
		return RateLimit{Blocked: true, NextSafeAt: oldest.Add(failedWindow), Reason: ReasonFailedValidations}
	}
	if oldest, n := within(s.Issued, now, duplicateWindow); n >= lim.DuplicatePerWeek {
		return RateLimit{Blocked: true, NextSafeAt: oldest.Add(duplicateWindow), Reason: ReasonDuplicates}
	}
	if IsRateLimitError(lastCAErr) {
		if until := lastCAErrAt.Add(rateLimitedPause); until.After(now) {
			return RateLimit{Blocked: true, NextSafeAt: until, Reason: ReasonCARateLimited}
		}
	}
	return RateLimit{}
}

func within(ts []time.Time, now time.Time, window time.Duration) (oldest time.Time, n int) {
	for _, t := range ts {
		if now.Sub(t) >= window {
			continue
		}
		if n == 0 || t.Before(oldest) {
			oldest = t
		}
		n++
	}
	return oldest, n
}

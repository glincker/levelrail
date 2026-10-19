// Package safemode keeps a boot ledger so a control plane that cannot start stops restarting and serves a read-only recovery page instead.
package safemode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Environment variables that tune boot failure handling.
const (
	EnvAfterFailures = "APP_SAFE_MODE_AFTER_FAILURES"
	EnvBackoffBase   = "APP_BOOT_BACKOFF_BASE"
	EnvBackoffMax    = "APP_BOOT_BACKOFF_MAX"
	EnvHealthyAfter  = "APP_BOOT_HEALTHY_AFTER"
)

// LedgerFilename is the boot ledger under the data dir.
const LedgerFilename = "boot-ledger.json"

// Reasons the control plane enters safe mode.
const (
	ReasonDBCorrupt       = "db_corrupt"
	ReasonMigrationFailed = "migration_failed"
	ReasonStoreFailed     = "store_open_failed"
	ReasonCrashLoop       = "crash_loop"
)

// State is what the ledger remembers between boots.
type State struct {
	Failures  int       `json:"failures"`
	Healthy   bool      `json:"healthy"`
	LastStart time.Time `json:"last_start"`
	LastError string    `json:"last_error,omitempty"`
}

// Ledger counts consecutive boots that never became healthy.
type Ledger struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
	st   State
}

// NewLedger returns a ledger stored under dataDir.
func NewLedger(dataDir string) *Ledger {
	return &Ledger{path: filepath.Join(dataDir, LedgerFilename), now: time.Now}
}

func (l *Ledger) load() State {
	var st State
	data, err := os.ReadFile(l.path) //nolint:gosec // fixed filename under the operator data dir
	if err != nil || json.Unmarshal(data, &st) != nil {
		return State{Healthy: true}
	}
	return st
}

func (l *Ledger) save() error {
	data, err := json.Marshal(l.st)
	if err != nil {
		return fmt.Errorf("encode boot ledger: %w", err)
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write boot ledger: %w", err)
	}
	if err := os.Rename(tmp, l.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace boot ledger: %w", err)
	}
	return nil
}

// Begin records a start. A previous start that never reached MarkHealthy or MarkClean counts as one more failure.
func (l *Ledger) Begin() State {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st = l.load()
	if !l.st.Healthy && !l.st.LastStart.IsZero() {
		l.st.Failures++
	}
	prev := l.st
	l.st.Healthy, l.st.LastStart = false, l.now().UTC()
	_ = l.save()
	return prev
}

// MarkHealthy records that this boot stayed up, which clears the failure count.
func (l *Ledger) MarkHealthy() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st.Healthy, l.st.Failures, l.st.LastError = true, 0, ""
	_ = l.save()
}

// MarkClean records a deliberate stop so it is not counted as a crash.
func (l *Ledger) MarkClean() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st.Healthy = true
	_ = l.save()
}

// ExitSafeMode records that safe mode ended without a recovery action. The next boot gets exactly one more attempt, so a manual restart after fixing the cause works and a broken install lands back in safe mode.
func (l *Ledger) ExitSafeMode(threshold int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st.Healthy = true
	l.st.Failures = max(0, threshold-1)
	_ = l.save()
}

// NoteError remembers why the last boot failed.
func (l *Ledger) NoteError(err error) {
	if err == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st.LastError = err.Error()
	_ = l.save()
}

// Reset forgets every failure, for a manual recovery.
func (l *Ledger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st = State{Healthy: true}
	_ = l.save()
}

// Policy turns a failure count into a delay and a safe mode decision.
type Policy struct {
	Threshold    int
	BackoffBase  time.Duration
	BackoffMax   time.Duration
	HealthyAfter time.Duration
}

// PolicyFromEnv reads the APP_SAFE_MODE_* and APP_BOOT_* overrides.
func PolicyFromEnv(getenv func(string) string) Policy {
	p := Policy{Threshold: 3, BackoffBase: 2 * time.Second, BackoffMax: time.Minute, HealthyAfter: 30 * time.Second}
	if n, err := strconv.Atoi(getenv(EnvAfterFailures)); err == nil && n > 0 {
		p.Threshold = n
	}
	for env, dst := range map[string]*time.Duration{EnvBackoffBase: &p.BackoffBase, EnvBackoffMax: &p.BackoffMax, EnvHealthyAfter: &p.HealthyAfter} {
		if d, err := time.ParseDuration(getenv(env)); err == nil && d > 0 {
			*dst = d
		}
	}
	return p
}

// Backoff is how long to wait before this boot continues: zero for a clean history, then exponential up to BackoffMax.
func (p Policy) Backoff(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := p.BackoffBase
	for i := 1; i < failures && d < p.BackoffMax; i++ {
		d *= 2
	}
	return min(d, p.BackoffMax)
}

// SafeMode reports whether failures have reached the point where restarting again is pointless.
func (p Policy) SafeMode(failures int) bool { return failures >= p.Threshold }

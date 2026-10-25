package reconcile

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EnvFullResync is how long an unchanged status may go without being written
// again. Zero or invalid falls back to the default.
const EnvFullResync = "APP_RECONCILE_FULL_RESYNC"

// EnvParallelism caps how many per-app controllers reconcile at once. 1 keeps
// the old sequential pass.
const EnvParallelism = "APP_RECONCILE_PARALLELISM"

const defaultParallelism = 4

// ParallelismFromEnv reads EnvParallelism.
func ParallelismFromEnv(getenv func(string) string) int {
	if n, err := strconv.Atoi(getenv(EnvParallelism)); err == nil && n > 0 {
		return n
	}
	return defaultParallelism
}

const defaultFullResync = 5 * time.Minute

// FullResyncFromEnv reads EnvFullResync.
func FullResyncFromEnv(getenv func(string) string) time.Duration {
	if getenv == nil {
		getenv = os.Getenv
	}
	if d, err := time.ParseDuration(getenv(EnvFullResync)); err == nil && d > 0 {
		return d
	}
	return defaultFullResync
}

type persistedEntry struct {
	sig string
	at  time.Time
}

// persistLedger remembers the last status written per controller so an
// unchanged status costs no database write until the full resync is due.
type persistLedger struct {
	mu      sync.Mutex
	entries map[string]persistedEntry
}

func conditionsSignature(conds []Condition) string {
	var b strings.Builder
	for _, c := range conds {
		b.WriteString(c.Type)
		b.WriteByte(0)
		b.WriteString(string(c.Status))
		b.WriteByte(0)
		b.WriteString(c.Reason)
		b.WriteByte(0)
		b.WriteString(c.Message)
		b.WriteByte(1)
	}
	return b.String()
}

// due reports whether conds must be written now, with the signature to record on success.
func (l *persistLedger) due(name string, conds []Condition, now time.Time, every time.Duration) (string, bool) {
	sig := conditionsSignature(conds)
	l.mu.Lock()
	defer l.mu.Unlock()
	prev, ok := l.entries[name]
	if !ok || prev.sig != sig || now.Sub(prev.at) >= every {
		return sig, true
	}
	return sig, false
}

func (l *persistLedger) record(name, sig string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries == nil {
		l.entries = map[string]persistedEntry{}
	}
	l.entries[name] = persistedEntry{sig: sig, at: now}
}

// forget drops names no longer reconciled so the ledger does not grow with deleted apps.
func (l *persistLedger) forget(live map[string]struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for name := range l.entries {
		if _, ok := live[name]; !ok {
			delete(l.entries, name)
		}
	}
}

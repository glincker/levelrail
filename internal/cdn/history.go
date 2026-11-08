package cdn

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// EnvPurgeHistory caps the purge records kept per app.
const EnvPurgeHistory = "APP_CDN_PURGE_HISTORY"

const defaultPurgeHistory = 20

// Purge triggers, so the history says why the cache was cleared.
const (
	TriggerPublish  = "publish"
	TriggerDeploy   = "deploy"
	TriggerRollback = "rollback"
)

// ScopeHosts is a purge of every URL under the app's hostnames.
const ScopeHosts = "hosts"

// PurgeRecord is one edge purge attempt for an app.
type PurgeRecord struct {
	At       time.Time `json:"at"`
	Trigger  string    `json:"trigger"`
	Scope    string    `json:"scope"`
	Provider string    `json:"provider"`
	Zones    []string  `json:"zones"`
	Hosts    int       `json:"hosts"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
}

// History keeps the newest purge records per app in memory.
type History struct {
	mu   sync.Mutex
	keep int
	byID map[string][]PurgeRecord
}

// NewHistory returns a history keeping keep records per app; keep <= 0 reads
// the environment.
func NewHistory(keep int) *History {
	if keep <= 0 {
		keep = defaultPurgeHistory
		if n, err := strconv.Atoi(os.Getenv(EnvPurgeHistory)); err == nil && n > 0 {
			keep = n
		}
	}
	return &History{keep: keep, byID: map[string][]PurgeRecord{}}
}

// SetKeep changes the per-app cap live; n <= 0 is ignored. Existing rows are trimmed on the next Add.
func (h *History) SetKeep(n int) {
	if n <= 0 {
		return
	}
	h.mu.Lock()
	h.keep = n
	h.mu.Unlock()
}

var defaultHistory = NewHistory(0)

// DefaultHistory is the process-wide history the API writes and reads.
func DefaultHistory() *History { return defaultHistory }

// Add stores r for app, newest first.
func (h *History) Add(app string, r PurgeRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := append([]PurgeRecord{r}, h.byID[app]...)
	if len(rows) > h.keep {
		rows = rows[:h.keep]
	}
	h.byID[app] = rows
}

// List returns up to limit records for app, newest first.
func (h *History) List(app string, limit int) []PurgeRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := h.byID[app]
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return append([]PurgeRecord{}, rows...)
}

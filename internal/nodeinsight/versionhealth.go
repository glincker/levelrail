package nodeinsight

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// VersionHealth remembers which agent version last proved healthy, so a bad
// update can name the version to go back to.
type VersionHealth struct {
	Current  string    `json:"current"`
	LastGood string    `json:"last_good,omitempty"`
	Failures int       `json:"failures"`
	Since    time.Time `json:"since"`
}

// Observe records the running version. A version change resets the failure
// count and keeps the previous version as the rollback target, unless that
// one was itself failing (then the older good version stays).
func (h *VersionHealth) Observe(version string, now time.Time) {
	if version == "" || version == h.Current {
		return
	}
	if h.Current != "" && h.Failures == 0 {
		h.LastGood = h.Current
	}
	h.Current, h.Failures, h.Since = version, 0, now
}

// RecordFailure counts one failed health check or short-lived session.
func (h *VersionHealth) RecordFailure() { h.Failures++ }

// RecordStable marks the current version good after a healthy stretch.
func (h *VersionHealth) RecordStable() {
	h.Failures = 0
	if h.Current != "" {
		h.LastGood = h.Current
	}
}

// LoadVersionHealth reads the state file; a missing file is an empty state.
func LoadVersionHealth(path string) (VersionHealth, error) {
	var h VersionHealth
	b, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path is the agent's own state file
	if errors.Is(err, os.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(b, &h)
}

// Save writes the state atomically so a crash never leaves half a file.
func (h VersionHealth) Save(path string) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// VersionTracker keeps one VersionHealth per node on the control plane. The
// zero value is ready; it is in memory and relearns after a restart.
type VersionTracker struct {
	mu    sync.Mutex
	nodes map[string]*VersionHealth
}

// Observe records a node's version and returns its state.
func (t *VersionTracker) Observe(id, version string, now time.Time) VersionHealth {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.nodes == nil {
		t.nodes = map[string]*VersionHealth{}
	}
	h := t.nodes[id]
	if h == nil {
		h = &VersionHealth{}
		t.nodes[id] = h
	}
	h.Observe(version, now)
	return *h
}

// OfflineSince counts transitions to offline at or after since.
func OfflineSince(events []Event, since time.Time) int {
	n := 0
	for _, e := range events {
		if e.To == "offline" && !e.At.Before(since) {
			n++
		}
	}
	return n
}

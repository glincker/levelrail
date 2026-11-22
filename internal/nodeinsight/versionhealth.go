package nodeinsight

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
	// RolledBackFrom is the version this node was returned from; it is not
	// offered again until the target changes or the hold is cleared.
	RolledBackFrom string `json:"rolled_back_from,omitempty"`
}

// Observe records the running version. A version change resets the failure
// count and keeps the previous version as the rollback target, unless that
// one was itself failing (then the older good version stays).
func (h *VersionHealth) Observe(version string, now time.Time) {
	if version == "" || version == h.Current {
		return
	}
	if h.Current != "" && h.LastGood == version && h.Current != version {
		h.RolledBackFrom = h.Current
	} else if h.Current != "" && h.Failures == 0 {
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

// ClearRolledBack lifts the hold for a node so the version can be offered again.
func (t *VersionTracker) ClearRolledBack(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if h := t.nodes[id]; h != nil {
		h.RolledBackFrom = ""
	}
}

// OfflineSince counts transitions to offline at or after since, but only after
// the node has reported online in that window. An offline event with no
// handshake before it is a control plane restart or a flap, not a bad version.
func OfflineSince(events []Event, since time.Time) int {
	ordered := make([]Event, 0, len(events))
	for _, e := range events {
		if !e.At.Before(since) {
			ordered = append(ordered, e)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })
	n, seenOnline := 0, false
	for _, e := range ordered {
		switch e.To {
		case "online":
			seenOnline = true
		case "offline":
			if seenOnline {
				n++
			}
		}
	}
	return n
}

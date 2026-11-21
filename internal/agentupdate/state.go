package agentupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	markerFile = "install-marker.json"
	failedFile = "update-failed.json"
)

// Marker records one install so a rollback is only ever judged against the
// version that was just installed, never against an older, healthy one.
type Marker struct {
	From        string    `json:"from"`
	To          string    `json:"to"`
	InstalledAt time.Time `json:"installed_at"`
}

// Failed remembers a target that did not work, so it is not retried in a loop.
type Failed struct {
	Target string    `json:"target"`
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadMarker returns the pending install marker, or false when there is none.
func ReadMarker(dir string) (Marker, bool) {
	var m Marker
	if err := readJSON(filepath.Join(dir, markerFile), &m); err != nil || m.To == "" {
		return Marker{}, false
	}
	return m, true
}

// WriteMarker stores the marker.
func WriteMarker(dir string, m Marker) error { return writeJSON(filepath.Join(dir, markerFile), m) }

// ClearMarker removes the marker; a missing file is fine.
func ClearMarker(dir string) {
	if err := os.Remove(filepath.Join(dir, markerFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
}

// ReadFailed returns the last failed target, or false.
func ReadFailed(dir string) (Failed, bool) {
	var f Failed
	if err := readJSON(filepath.Join(dir, failedFile), &f); err != nil || f.Target == "" {
		return Failed{}, false
	}
	return f, true
}

// WriteFailed records a failed target.
func WriteFailed(dir string, f Failed) error { return writeJSON(filepath.Join(dir, failedFile), f) }

// InProbation reports whether the marker still guards a rollback decision for
// the running version.
func (m Marker) InProbation(running string, window time.Duration, now time.Time) bool {
	return m.To != "" && m.To == running && now.Sub(m.InstalledAt) <= window
}

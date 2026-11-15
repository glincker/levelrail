// Package runmarker detects an unclean stop of the control plane. A marker
// file written at boot and marked clean at graceful shutdown tells the next
// boot whether the previous run was killed, and the kernel's oom_kill counter
// says whether the host killed it for memory.
package runmarker

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	markerFile  = "run-marker.json"
	historyFile = "unclean-restarts.json"
	procVmstat  = "/proc/vmstat"
	maxHistory  = 20

	// EnvWindow sets how long an unclean restart stays in the attention feed.
	EnvWindow     = "APP_UNCLEAN_RESTART_WINDOW"
	defaultWindow = 72 * time.Hour
)

// Marker is the small file the running process leaves behind. OOMKill is -1
// when the counter could not be read.
type Marker struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	OOMKill   int64     `json:"oom_kill"`
	Clean     bool      `json:"clean"`
}

// Incident is one boot that found the previous run had not stopped cleanly.
type Incident struct {
	At            time.Time `json:"at"`
	PrevStartedAt time.Time `json:"prev_started_at"`
	PrevPID       int       `json:"prev_pid"`
	OOMLikely     bool      `json:"oom_likely"`
}

// Tracker owns the marker for the current process.
type Tracker struct {
	dir string
	cur Marker
}

// WindowFromEnv reads EnvWindow as a duration, falling back to three days.
func WindowFromEnv(getenv func(string) string) time.Duration {
	if d, err := time.ParseDuration(getenv(EnvWindow)); err == nil && d > 0 {
		return d
	}
	return defaultWindow
}

// OOMKills reads the oom_kill counter from a vmstat file. ok is false when
// the file is missing or has no such line, so callers degrade quietly.
func OOMKills(path string) (n int64, ok bool) {
	raw, err := os.ReadFile(path) //nolint:gosec // fixed /proc path or a test fixture
	if err != nil {
		return 0, false
	}
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		key, val, found := bytes.Cut(line, []byte{' '})
		if !found || string(key) != "oom_kill" {
			continue
		}
		v, err := strconv.ParseInt(string(bytes.TrimSpace(val)), 10, 64)
		return v, err == nil
	}
	return 0, false
}

// Begin reads the previous marker, records an incident when it was not clean
// and writes a fresh marker for this run. It returns the incident or nil.
func Begin(dir string, now time.Time) (*Tracker, *Incident) {
	return begin(dir, procVmstat, now)
}

func begin(dir, vmstat string, now time.Time) (*Tracker, *Incident) {
	cur := Marker{PID: os.Getpid(), StartedAt: now.UTC(), OOMKill: -1}
	if n, ok := OOMKills(vmstat); ok {
		cur.OOMKill = n
	}
	t := &Tracker{dir: dir, cur: cur}
	var inc *Incident
	var prev Marker
	raw, rerr := os.ReadFile(filepath.Join(dir, markerFile)) //nolint:gosec // fixed name under the data dir
	if rerr == nil && json.Unmarshal(raw, &prev) == nil && !prev.Clean {
		inc = &Incident{
			At: now.UTC(), PrevStartedAt: prev.StartedAt, PrevPID: prev.PID,
			OOMLikely: prev.OOMKill >= 0 && cur.OOMKill > prev.OOMKill,
		}
		appendIncident(dir, *inc)
	}
	t.write()
	return t, inc
}

// Clean marks the run as stopped on purpose. Safe on a nil Tracker.
func (t *Tracker) Clean() {
	if t == nil {
		return
	}
	t.cur.Clean = true
	t.write()
}

func (t *Tracker) write() { writeJSON(filepath.Join(t.dir, markerFile), t.cur) }

// Recent returns incidents newer than window, newest first.
func Recent(dir string, window time.Duration, now time.Time) []Incident {
	var all []Incident
	raw, err := os.ReadFile(filepath.Join(dir, historyFile)) //nolint:gosec // fixed name under the data dir
	if err != nil || json.Unmarshal(raw, &all) != nil {
		return nil
	}
	var out []Incident
	for i := len(all) - 1; i >= 0; i-- {
		if now.Sub(all[i].At) <= window {
			out = append(out, all[i])
		}
	}
	return out
}

func appendIncident(dir string, inc Incident) {
	var all []Incident
	if raw, err := os.ReadFile(filepath.Join(dir, historyFile)); err == nil { //nolint:gosec // fixed name under the data dir
		_ = json.Unmarshal(raw, &all)
	}
	all = append(all, inc)
	if len(all) > maxHistory {
		all = all[len(all)-maxHistory:]
	}
	writeJSON(filepath.Join(dir, historyFile), all)
}

// writeJSON is best effort: a read-only data dir must not stop the boot.
func writeJSON(path string, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

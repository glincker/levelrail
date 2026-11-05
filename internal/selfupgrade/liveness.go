package selfupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrCancelled is the cause recorded when an operator cancels a running attempt.
var ErrCancelled = errors.New("cancelled by an operator before the release was applied")

// Heartbeat is what a live worker writes beside its journal file. It is a
// separate file so the worker's goroutine never races the attempt itself.
type Heartbeat struct {
	At         time.Time `json:"at"`
	Step       string    `json:"step,omitempty"`
	Bytes      int64     `json:"bytes"`
	Total      int64     `json:"total"`
	ProgressAt time.Time `json:"progress_at,omitempty"`
}

func (j *Journal) hbPath(id string) string     { return filepath.Join(j.Dir, id+".hb") }
func (j *Journal) cancelPath(id string) string { return filepath.Join(j.Dir, id+".cancel") }

// WriteHeartbeat stores hb for attempt id.
func (j *Journal) WriteHeartbeat(id string, hb Heartbeat) error {
	if !validID(id) {
		return fmt.Errorf("invalid attempt id %q", id)
	}
	raw, err := json.Marshal(hb)
	if err != nil {
		return err
	}
	tmp := j.hbPath(id) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write heartbeat: %w", err)
	}
	return os.Rename(tmp, j.hbPath(id))
}

// ReadHeartbeat returns the last heartbeat of id, or false when none exists.
func (j *Journal) ReadHeartbeat(id string) (Heartbeat, bool) {
	if !validID(id) {
		return Heartbeat{}, false
	}
	raw, err := os.ReadFile(j.hbPath(id)) //nolint:gosec // validated id inside the journal directory
	if err != nil {
		return Heartbeat{}, false
	}
	var hb Heartbeat
	if json.Unmarshal(raw, &hb) != nil {
		return Heartbeat{}, false
	}
	return hb, true
}

// RequestCancel asks the worker of attempt id to stop. The worker honours it
// only before the release is applied.
func (j *Journal) RequestCancel(id string) error {
	if !validID(id) {
		return fmt.Errorf("invalid attempt id %q", id)
	}
	if err := os.MkdirAll(j.Dir, 0o700); err != nil {
		return fmt.Errorf("create journal directory: %w", err)
	}
	return os.WriteFile(j.cancelPath(id), []byte("cancel\n"), 0o600)
}

// CancelRequested reports whether a cancel request is pending for id.
func (j *Journal) CancelRequested(id string) bool {
	if !validID(id) {
		return false
	}
	_, err := os.Stat(j.cancelPath(id))
	return err == nil
}

// ClearSignals removes the heartbeat and cancel files of id.
func (j *Journal) ClearSignals(id string) {
	if validID(id) {
		_ = os.Remove(j.hbPath(id))
		_ = os.Remove(j.cancelPath(id))
	}
}

// Cancellable reports whether a step can still be abandoned without touching
// the host: everything before the service is stopped.
func Cancellable(step string) bool {
	switch step {
	case StepStop, StepSwap, StepStart, StepHealth, StepRollback:
		return false
	}
	return true
}

// pastPoint reports whether the attempt reached a step that changes the host.
func pastPoint(a Attempt) bool {
	if !Cancellable(a.Current) {
		return true
	}
	for _, s := range a.Steps {
		if !Cancellable(s.Name) {
			return true
		}
	}
	return false
}

// LastSignal is the newest sign of life of a running attempt.
func LastSignal(j *Journal, a Attempt) time.Time {
	last := a.StartedAt
	if hb, ok := j.ReadHeartbeat(a.ID); ok && hb.At.After(last) {
		last = hb.At
	}
	if n := len(a.Log); n > 0 && a.Log[n-1].At.After(last) {
		last = a.Log[n-1].At
	}
	return last
}

// Orphaned reports whether a running attempt has no live worker behind it.
func Orphaned(j *Journal, a Attempt, now time.Time, l Limits) bool {
	return a.Outcome == OutcomeRunning && now.Sub(LastSignal(j, a)) > l.OrphanAfter
}

// ReapOrphans closes running attempts whose worker is gone and that never
// reached a step that changes the host, removing their partial downloads.
// Attempts past that point need "self-upgrade --recover" and are left alone.
// It returns the attempts it closed.
func ReapOrphans(ctx context.Context, j *Journal, workDir string, now time.Time, l Limits) ([]Attempt, error) {
	running, err := j.Running()
	if err != nil {
		return nil, err
	}
	var closed []Attempt
	for _, a := range running {
		if ctx.Err() != nil {
			return closed, ctx.Err()
		}
		if !Orphaned(j, a, now, l) || pastPoint(a) {
			continue
		}
		step := a.Current
		if step == "" {
			step = a.lastStep()
		}
		cause := fmt.Errorf("the upgrade process stopped without finishing (no sign of life for %s). Nothing on this host was changed. Retry the upgrade", now.Sub(LastSignal(j, a)).Round(time.Second))
		if j.CancelRequested(a.ID) {
			cause = ErrCancelled
		}
		a.Steps = append(a.Steps, StepRecord{Name: step, Status: StatusFailed, Detail: cause.Error(), At: now})
		a.AddLog(now, step, "failed: "+cause.Error())
		a.Outcome, a.FailedStep, a.Error, a.Current, a.FinishedAt = OutcomeFailed, step, cause.Error(), "", now
		if err := j.Save(a); err != nil {
			return closed, err
		}
		j.ClearSignals(a.ID)
		if workDir != "" {
			_ = os.RemoveAll(workDir)
		}
		closed = append(closed, a)
	}
	return closed, nil
}

// WorkDirName is the directory under the data dir that holds a download in progress.
const WorkDirName = "self-upgrade-work"

// ReapAndImport closes abandoned attempts, then copies the journal into the
// database so the dashboard sees the failure. Used at boot and before every
// status read.
func ReapAndImport(ctx context.Context, dataDir string, db AttemptStore, now time.Time, l Limits) ([]Attempt, error) {
	j := NewJournal(dataDir)
	closed, err := ReapOrphans(ctx, j, filepath.Join(dataDir, WorkDirName), now, l)
	if err != nil {
		return closed, err
	}
	if _, err := ImportJournal(ctx, j, db); err != nil {
		return closed, err
	}
	return closed, nil
}

// CanCancel reports whether a running attempt can still be abandoned without
// touching the host.
func CanCancel(a Attempt) bool {
	return a.Outcome == OutcomeRunning && !pastPoint(a)
}

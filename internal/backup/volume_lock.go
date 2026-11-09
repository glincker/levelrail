package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrVolumeBackupBusy is returned when a backup of the same volume is already
// running: a second run's post-hook could thaw the first one mid-archive.
var ErrVolumeBackupBusy = errors.New("another backup of this volume is running")

// volumeLockSet is a per (service, volume) try-lock shared by the manual
// trigger and the scheduler, since both call RunVolumeBackup on one Runner.
type volumeLockSet struct {
	mu   sync.Mutex
	held map[string]struct{}
}

func volumeLockKey(service, volume string) string { return service + "\x00" + volume }

func (l *volumeLockSet) tryLock(service, volume string) (release func(), ok bool) {
	k := volumeLockKey(service, volume)
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, busy := l.held[k]; busy {
		return nil, false
	}
	if l.held == nil {
		l.held = map[string]struct{}{}
	}
	l.held[k] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			delete(l.held, k)
			l.mu.Unlock()
		})
	}, true
}

func (l *volumeLockSet) running(service, volume string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, busy := l.held[volumeLockKey(service, volume)]
	return busy
}

var runnerLocksMu sync.Mutex

// locks is lazy because Runners are built as struct literals; copies made
// after the first backup share the same set.
func (r *Runner) locks() *volumeLockSet {
	runnerLocksMu.Lock()
	defer runnerLocksMu.Unlock()
	if r.volumeLocks == nil {
		r.volumeLocks = &volumeLockSet{}
	}
	return r.volumeLocks
}

// VolumeBackupRunning lets the API answer 409 before it records a new run.
func (r *Runner) VolumeBackupRunning(service, volume string) bool {
	return r.locks().running(service, volume)
}

// recordVolumeBusy still writes a failed row, because the manual trigger has
// already handed its history ID to the caller.
func (r *Runner) recordVolumeBusy(ctx context.Context, historyID, service, volume, targetID string) error {
	now := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.StartBackupHistory(ctx, store.BackupHistory{
		ID: historyID, ResourceKind: store.BackupResourceKindVolume, ServiceName: service, VolumeName: volume,
		TargetID: targetID, StartedAt: now,
	}); err != nil {
		return fmt.Errorf("backup: start history %q: %w", historyID, err)
	}
	if err := r.Store.FinishBackupHistory(ctx, historyID, store.BackupStatusFailed, 0, "", ErrVolumeBackupBusy.Error(), now); err != nil {
		return fmt.Errorf("backup: finish history %q: %w", historyID, err)
	}
	return ErrVolumeBackupBusy
}

// HookOutputStore keeps a failing hook's masked output apart from the
// history row, so only callers allowed to see hooks can read it.
type HookOutputStore interface {
	SaveBackupHookOutput(ctx context.Context, historyID, phase, output string) error
}

func (r *Runner) saveHookOutput(ctx context.Context, historyID string, runErr error) {
	var he *HookError
	if !errors.As(runErr, &he) || he.Output == "" {
		return
	}
	hs, ok := r.Store.(HookOutputStore)
	if !ok {
		return
	}
	if err := hs.SaveBackupHookOutput(context.WithoutCancel(ctx), historyID, he.Phase, he.Output); err != nil && r.Volume != nil && r.Volume.Logger != nil {
		r.Volume.Logger.Warn("backup: save hook output failed", slog.String("id", historyID), slog.String("error", err.Error()))
	}
}

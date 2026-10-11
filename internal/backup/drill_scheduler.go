package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultDrillInterval is how often each resource is drilled when
// APP_BACKUP_DRILL_INTERVAL_HOURS is unset.
const DefaultDrillInterval = 7 * 24 * time.Hour

// DefaultStaleRunning is how long a backup may stay running before the sweep
// examines it.
const DefaultStaleRunning = 6 * time.Hour

const protectionInterval = 24 * time.Hour

// DrillSchedulerStore is the store surface the drill scheduler needs.
type DrillSchedulerStore interface {
	backupResolver
	ListSucceededBackups(ctx context.Context) ([]store.BackupHistory, error)
	ListBackupDrills(ctx context.Context, serviceName, volumeName, databaseName string, limit int) ([]store.BackupDrill, error)
	ListRunningBackups(ctx context.Context, startedBefore time.Time) ([]store.BackupHistory, error)
	FinishBackupHistory(ctx context.Context, id, status string, sizeBytes int64, checksum, errMsg, finishedAt string) error
	ListBackupTargets(ctx context.Context) ([]store.BackupTarget, error)
	SetBackupTargetProtection(ctx context.Context, p store.BackupTargetProtection) error
}

// DrillScheduler drills every backed up resource on an interval, finalizes
// backups the control plane lost track of, and refreshes bucket protection
// probes.
type DrillScheduler struct {
	Store      DrillSchedulerStore
	Secrets    SecretsResolver
	Runner     *DrillRunner
	Prober     ObjectProber
	Downloader Downloader
	Protection ProtectionProber
	// Interval is the drill period per resource; zero disables drills.
	Interval time.Duration
	// StaleAfter is the age a running backup must reach before the sweep
	// finalizes it.
	StaleAfter time.Duration
	Logger     *slog.Logger
	Now        func() time.Time

	lastProtection time.Time
}

// DrillIntervalFromEnv parses APP_BACKUP_DRILL_INTERVAL_HOURS: unset means
// the default, 0 disables scheduled drills.
func DrillIntervalFromEnv(lookup func(string) (string, bool)) time.Duration {
	v, ok := lookup(EnvBackupDrillInterval)
	if !ok || strings.TrimSpace(v) == "" {
		return DefaultDrillInterval
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return DefaultDrillInterval
	}
	return time.Duration(n) * time.Hour
}

// StaleRunningFromEnv parses APP_BACKUP_STALE_RUNNING_MINUTES.
func StaleRunningFromEnv(lookup func(string) (string, bool)) time.Duration {
	if v, ok := lookup(EnvBackupStaleMinutes); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return DefaultStaleRunning
}

func (s *DrillScheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *DrillScheduler) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// Tick runs one scheduling pass.
func (s *DrillScheduler) Tick(ctx context.Context) error {
	var errs []error
	if err := s.sweepInterrupted(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.refreshProtection(ctx); err != nil {
		errs = append(errs, err)
	}
	if s.Interval > 0 && s.Runner != nil {
		if err := s.drillDue(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Run ticks until ctx is done.
func (s *DrillScheduler) Run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := s.Tick(ctx); err != nil {
				s.log().Error("backup: drill scheduler tick failed", slog.String("error", err.Error()))
			}
		}
	}
}

type drillKey struct{ kind, a, b string }

func keyOf(h store.BackupHistory) drillKey {
	if h.ResourceKind == store.BackupResourceKindVolume {
		return drillKey{h.ResourceKind, h.ServiceName, h.VolumeName}
	}
	return drillKey{h.ResourceKind, h.DatabaseName, ""}
}

func (s *DrillScheduler) drillDue(ctx context.Context) error {
	rows, err := s.Store.ListSucceededBackups(ctx)
	if err != nil {
		return fmt.Errorf("list backups for drills: %w", err)
	}
	newest := map[drillKey]store.BackupHistory{}
	for _, h := range rows {
		k := keyOf(h)
		if cur, ok := newest[k]; !ok || h.StartedAt > cur.StartedAt {
			newest[k] = h
		}
	}
	var errs []error
	for k, h := range newest {
		if ctx.Err() != nil {
			break
		}
		var drills []store.BackupDrill
		var lerr error
		if k.kind == store.BackupResourceKindVolume {
			drills, lerr = s.Store.ListBackupDrills(ctx, k.a, k.b, "", 1)
		} else {
			drills, lerr = s.Store.ListBackupDrills(ctx, "", "", k.a, 1)
		}
		if lerr != nil {
			errs = append(errs, lerr)
			continue
		}
		if len(drills) > 0 {
			if at, perr := time.Parse(time.RFC3339, drills[0].StartedAt); perr == nil && s.now().Sub(at) < s.Interval {
				continue
			}
		}
		id, ierr := NewDrillID()
		if ierr != nil {
			errs = append(errs, ierr)
			continue
		}
		if _, derr := s.Runner.RunDrill(ctx, id, h.ID, store.DrillTriggerScheduled); derr != nil {
			errs = append(errs, derr)
		}
	}
	return errors.Join(errs...)
}

// NewDrillID mints a drill identifier safe to embed in a Docker name.
func NewDrillID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("backup: generate drill id: %w", err)
	}
	return "bkd" + strings.NewReplacer("-", "x", "_", "y").Replace(base64.RawURLEncoding.EncodeToString(buf)), nil
}

// sweepInterrupted settles backups left running by a crash. An object that
// made it to the bucket is hashed and recorded as succeeded (multipart
// upload completes atomically, so presence means it is whole); a missing one
// is marked failed.
func (s *DrillScheduler) sweepInterrupted(ctx context.Context) error {
	stale := s.StaleAfter
	if stale <= 0 {
		stale = DefaultStaleRunning
	}
	rows, err := s.Store.ListRunningBackups(ctx, s.now().Add(-stale))
	if err != nil {
		return fmt.Errorf("list running backups: %w", err)
	}
	var errs []error
	for _, h := range rows {
		if err := s.settle(ctx, h); err != nil {
			errs = append(errs, fmt.Errorf("settle backup %q: %w", h.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *DrillScheduler) settle(ctx context.Context, h store.BackupHistory) error {
	finished := s.now().UTC().Format(time.RFC3339)
	dest, err := resolveTargetDestination(ctx, s.Store, s.Secrets, h.TargetID)
	if err != nil {
		return s.Store.FinishBackupHistory(ctx, h.ID, store.BackupStatusFailed, 0, "", "interrupted: backup target is not reachable to settle it: "+err.Error(), finished)
	}
	info, err := s.Prober.Head(ctx, dest, h.ObjectKey)
	if err != nil {
		return fmt.Errorf("check object: %w", err)
	}
	if !info.Exists {
		return s.Store.FinishBackupHistory(ctx, h.ID, store.BackupStatusFailed, 0, "", "interrupted: the control plane stopped before this upload finished and no object was stored", finished)
	}
	rc, err := s.Downloader.Download(ctx, dest, h.ObjectKey)
	if err != nil {
		return fmt.Errorf("download for checksum: %w", err)
	}
	defer func() { _ = rc.Close() }()
	hash := sha256.New()
	n, err := io.Copy(hash, rc)
	if err != nil {
		return fmt.Errorf("hash object: %w", err)
	}
	return s.Store.FinishBackupHistory(ctx, h.ID, store.BackupStatusSucceeded, n, hex.EncodeToString(hash.Sum(nil)), "", finished)
}

func (s *DrillScheduler) refreshProtection(ctx context.Context) error {
	if s.Protection == nil || (!s.lastProtection.IsZero() && s.now().Sub(s.lastProtection) < protectionInterval) {
		return nil
	}
	s.lastProtection = s.now()
	targets, err := s.Store.ListBackupTargets(ctx)
	if err != nil {
		return fmt.Errorf("list backup targets: %w", err)
	}
	for _, t := range targets {
		RecordProtection(ctx, s.Store, s.Secrets, s.Protection, t.ID, s.now())
	}
	return nil
}

// ProtectionRecorder is the slice of the store RecordProtection writes to.
type ProtectionRecorder interface {
	backupResolver
	SetBackupTargetProtection(ctx context.Context, p store.BackupTargetProtection) error
}

// RecordProtection probes one target's bucket and stores the result. A probe
// that cannot run is stored with its error so the dashboard can say so.
func RecordProtection(ctx context.Context, st ProtectionRecorder, secrets SecretsResolver, prober ProtectionProber, targetID string, at time.Time) store.BackupTargetProtection {
	rec := store.BackupTargetProtection{TargetID: targetID, CheckedAt: at.UTC().Format(time.RFC3339)}
	dest, err := resolveTargetDestination(ctx, st, secrets, targetID)
	if err == nil {
		var p BucketProtection
		p, err = prober.Probe(ctx, dest)
		rec.ObjectLock, rec.LockMode, rec.Versioning, rec.CanDelete = p.ObjectLock, p.LockMode, p.Versioning, p.CanDelete
	}
	if err != nil {
		rec.ProbeError = err.Error()
	}
	_ = st.SetBackupTargetProtection(ctx, rec)
	return rec
}

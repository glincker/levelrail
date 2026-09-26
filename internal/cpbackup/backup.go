package cpbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// runTimeout bounds one background backup or drill.
const runTimeout = 2 * time.Hour

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) tempDir() (string, error) {
	parent := filepath.Join(s.DataDir, DirName)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	dir, err := os.MkdirTemp(parent, ".offbox-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return dir, nil
}

// CleanTemp removes work directories a crashed run left behind. They can hold a
// plaintext snapshot, so it runs at startup when no run can be in progress.
func (s *Service) CleanTemp() {
	matches, err := filepath.Glob(filepath.Join(s.DataDir, DirName, ".offbox-*"))
	if err != nil {
		return
	}
	for _, m := range matches {
		if err := os.RemoveAll(m); err != nil {
			s.logger().Warn("remove stale off-box work directory", slog.String("path", m), slog.String("error", err.Error()))
		}
	}
}

// RunBackup snapshots the database, encrypts it to the configured recipients
// and uploads it with a manifest. slot names the schedule tick (zero for a
// manual run): a slot whose manifest already exists is skipped, and one whose
// upload was cut short is redone over the same keys.
func (s *Service) RunBackup(ctx context.Context, slot time.Time) (Manifest, error) {
	if !s.backupMu.TryLock() {
		return Manifest{}, ErrBusy
	}
	defer s.backupMu.Unlock()
	return s.backupLocked(ctx, slot)
}

// StartBackup begins a manual backup in the background and returns at once.
// ErrBusy means one is already running.
func (s *Service) StartBackup() error {
	if !s.backupMu.TryLock() {
		return ErrBusy
	}
	go func() {
		defer s.backupMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
		defer cancel()
		if _, err := s.backupLocked(ctx, time.Time{}); err != nil {
			s.logger().Error("manual off-box control plane backup failed", slog.String("error", err.Error()))
		}
	}()
	return nil
}

// BackupRunning reports whether a backup is in progress.
func (s *Service) BackupRunning() bool {
	if s.backupMu.TryLock() {
		s.backupMu.Unlock()
		return false
	}
	return true
}

func (s *Service) backupLocked(ctx context.Context, slot time.Time) (Manifest, error) {
	started := s.now().UTC().Truncate(time.Second)
	m, err := s.runBackup(ctx, slot, started)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			if recErr := s.Store.RecordCPDRBackup(context.WithoutCancel(ctx), started, "", err.Error()); recErr != nil {
				s.logger().Error("record off-box backup failure", slog.String("error", recErr.Error()))
			}
		}
		return Manifest{}, err
	}
	if recErr := s.Store.RecordCPDRBackup(ctx, started, m.Key, ""); recErr != nil {
		return m, fmt.Errorf("record off-box backup: %w", recErr)
	}
	return m, nil
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func (s *Service) runBackup(ctx context.Context, slot, started time.Time) (Manifest, error) {
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return Manifest{}, fmt.Errorf("load settings: %w", err)
	}
	if cfg.TargetID == "" || len(cfg.Recipients) == 0 {
		return Manifest{}, ErrNotConfigured
	}
	recipients, err := ParseRecipients(s.recipients(cfg))
	if err != nil {
		return Manifest{}, fmt.Errorf("recipients: %w", err)
	}
	installID, err := s.installID(ctx, cfg)
	if err != nil {
		return Manifest{}, err
	}
	bucket, _, err := s.Dest.Open(ctx, cfg.TargetID)
	if err != nil {
		return Manifest{}, fmt.Errorf("open destination: %w", err)
	}

	if slot.IsZero() {
		slot = started
	}
	dataKey := DataKey(installID, slot)
	if existing, err := FetchManifest(ctx, bucket, dataKey); err == nil && confirmObject(ctx, bucket, dataKey, existing.SizeBytes) == nil {
		return existing, nil
	}

	tmp, err := s.tempDir()
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	snapPath := filepath.Join(tmp, "snapshot.db")
	if _, _, err := s.DB.SnapshotTo(ctx, snapPath); err != nil {
		return Manifest{}, fmt.Errorf("snapshot: %w", err)
	}
	version, err := store.InspectSnapshot(ctx, snapPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("inspect snapshot: %w", err)
	}
	migrations, err := inspectMigrationCount(ctx, snapPath)
	if err != nil {
		return Manifest{}, err
	}
	cipherPath := filepath.Join(tmp, "snapshot.db.age")
	sl, err := seal(snapPath, cipherPath, recipients)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{
		ManifestVersion: manifestFormat, InstallID: installID, Key: dataKey, CreatedAt: started,
		BinaryVersion: s.BinaryVersion, SchemaVersion: version, MigrationsApplied: migrations,
		SHA256: sl.SHA256, SizeBytes: sl.Size, PlainSHA256: sl.PlainSHA256, PlainSizeBytes: sl.PlainSize,
		Compression: "gzip", Encryption: "age", RecipientCount: len(recipients),
		ContainsWrappedSecrets: true, IncludesMasterKey: false,
	}
	if err := upload(ctx, bucket, dataKey, cipherPath, m); err != nil {
		return Manifest{}, err
	}

	ret := s.effective(cfg).Retention
	if n, err := pruneRemote(ctx, bucket, installPrefix(installID), ret, s.Opts.MaxPrune, s.Opts.OrphanAge, started); err != nil {
		s.logger().Warn("prune off-box control plane backups failed", slog.String("error", err.Error()))
	} else if n > 0 {
		s.logger().Info("pruned off-box control plane backups", slog.Int("removed", n))
	}
	return m, nil
}

// upload sends the ciphertext, checks its stored size, then commits by writing the manifest.
func confirmObject(ctx context.Context, b Bucket, key string, size int64) error {
	page, err := b.List(ctx, key, "", 1)
	if err != nil {
		return err
	}
	if len(page.Objects) == 0 || page.Objects[0].Key != key || page.Objects[0].Size != size {
		return errors.New("stored object does not match what was sent")
	}
	return nil
}

func upload(ctx context.Context, b Bucket, dataKey, cipherPath string, m Manifest) error {
	f, err := os.Open(cipherPath) //nolint:gosec // path built by this package
	if err != nil {
		return fmt.Errorf("open encrypted backup: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := b.Put(ctx, dataKey, f, "application/octet-stream", ""); err != nil {
		return fmt.Errorf("upload backup: %w", err)
	}
	if err := confirmObject(ctx, b, dataKey, m.SizeBytes); err != nil {
		return fmt.Errorf("confirm upload: %w", err)
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := b.Put(ctx, ManifestKey(dataKey), bytes.NewReader(body), "application/json", ""); err != nil {
		return fmt.Errorf("upload manifest: %w", err)
	}
	return nil
}

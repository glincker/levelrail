// Package cpbackup manages control plane self-backups: verified SQLite
// snapshots kept in a directory under the data dir.
package cpbackup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DirName is the backup directory's name under the data dir.
const DirName = "control-plane-backups"

const (
	nameLayout      = "20060102T150405Z"
	scheduledSuffix = ".scheduled"
)

var namePattern = regexp.MustCompile(`^levelrail-(\d{8}T\d{6}Z)\.db$`)

var (
	// ErrInvalidName means the name does not match the backup file pattern.
	ErrInvalidName = errors.New("invalid backup name")
	// ErrNotFound means no backup with that name exists.
	ErrNotFound = errors.New("backup not found")
)

// Info describes one backup.
type Info struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	SHA256    string    `json:"sha256"`
}

// Snapshotter is the slice of *store.DB the manager needs.
type Snapshotter interface {
	SnapshotTo(ctx context.Context, dest string) (int64, string, error)
}

// Manager creates, lists, and prunes backups in one directory.
type Manager struct {
	db  Snapshotter
	dir string
	now func() time.Time
}

// NewManager returns a Manager storing backups under dataDir/DirName.
func NewManager(db Snapshotter, dataDir string) *Manager {
	return &Manager{db: db, dir: filepath.Join(dataDir, DirName), now: time.Now}
}

// ValidName reports whether name is a well-formed backup file name.
func ValidName(name string) bool { return namePattern.MatchString(name) }

// Create takes a manual snapshot, which is never auto-pruned.
func (m *Manager) Create(ctx context.Context) (Info, error) {
	return m.create(ctx, false)
}

// CreateScheduled takes a snapshot that Prune may later remove.
func (m *Manager) CreateScheduled(ctx context.Context) (Info, error) {
	return m.create(ctx, true)
}

func (m *Manager) create(ctx context.Context, scheduled bool) (Info, error) {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return Info{}, fmt.Errorf("create backup dir: %w", err)
	}
	ts := m.now().UTC().Truncate(time.Second)
	var name string
	for i := 0; i < 60; i++ {
		name = "levelrail-" + ts.Format(nameLayout) + ".db"
		if _, err := os.Stat(filepath.Join(m.dir, name)); os.IsNotExist(err) {
			break
		}
		ts = ts.Add(time.Second)
	}
	path := filepath.Join(m.dir, name)
	size, sum, err := m.db.SnapshotTo(ctx, path)
	if err != nil {
		return Info{}, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return Info{}, fmt.Errorf("chmod backup: %w", err)
	}
	if scheduled {
		if err := os.WriteFile(path+scheduledSuffix, nil, 0o600); err != nil {
			return Info{}, fmt.Errorf("mark backup scheduled: %w", err)
		}
	}
	return Info{Name: name, SizeBytes: size, CreatedAt: ts, SHA256: sum}, nil
}

// List returns every backup, newest first.
func (m *Manager) List() ([]Info, error) {
	entries, err := os.ReadDir(m.dir)
	if os.IsNotExist(err) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	out := []Info{}
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		info, err := m.stat(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Newest returns the creation time of the most recent backup, read from file
// names alone (no checksumming), and false when there is none.
func (m *Manager) Newest() (time.Time, bool, error) {
	entries, err := os.ReadDir(m.dir)
	if os.IsNotExist(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read backup dir: %w", err)
	}
	var newest time.Time
	for _, e := range entries {
		match := namePattern.FindStringSubmatch(e.Name())
		if e.IsDir() || match == nil {
			continue
		}
		ts, err := time.Parse(nameLayout, match[1])
		if err != nil {
			continue
		}
		if ts.After(newest) {
			newest = ts
		}
	}
	return newest.UTC(), !newest.IsZero(), nil
}

func (m *Manager) stat(name string) (Info, error) {
	size, sum, err := store.FileSHA256(filepath.Join(m.dir, name))
	if err != nil {
		return Info{}, err
	}
	created, err := time.Parse(nameLayout, namePattern.FindStringSubmatch(name)[1])
	if err != nil {
		return Info{}, fmt.Errorf("parse backup name %q: %w", name, err)
	}
	return Info{Name: name, SizeBytes: size, CreatedAt: created.UTC(), SHA256: sum}, nil
}

// Open returns the backup file for reading.
func (m *Manager) Open(name string) (*os.File, Info, error) {
	if !ValidName(name) {
		return nil, Info{}, ErrInvalidName
	}
	f, err := os.Open(filepath.Join(m.dir, name)) //nolint:gosec // name validated against namePattern
	if os.IsNotExist(err) {
		return nil, Info{}, ErrNotFound
	}
	if err != nil {
		return nil, Info{}, fmt.Errorf("open backup: %w", err)
	}
	info, err := m.stat(name)
	if err != nil {
		_ = f.Close()
		return nil, Info{}, err
	}
	return f, info, nil
}

// Delete removes one backup.
func (m *Manager) Delete(name string) error {
	if !ValidName(name) {
		return ErrInvalidName
	}
	path := filepath.Join(m.dir, name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete backup: %w", err)
	}
	_ = os.Remove(path + scheduledSuffix)
	return nil
}

// Prune deletes the oldest scheduled backups beyond retain. Manual backups
// are never touched.
func (m *Manager) Prune(retain int) (int, error) {
	if retain < 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(m.dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read backup dir: %w", err)
	}
	var scheduled []string
	for _, e := range entries {
		if !ValidName(e.Name()) {
			continue
		}
		if _, err := os.Stat(filepath.Join(m.dir, e.Name()+scheduledSuffix)); err == nil {
			scheduled = append(scheduled, e.Name())
		}
	}
	sort.Strings(scheduled)
	removed := 0
	for len(scheduled)-removed > retain {
		if err := m.Delete(scheduled[removed]); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Run takes a scheduled snapshot every interval and prunes to retain. It
// returns when ctx is done.
func (m *Manager) Run(ctx context.Context, interval time.Duration, retain int, logger *slog.Logger) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			info, err := m.CreateScheduled(ctx)
			if err != nil {
				logger.Error("scheduled control plane backup failed", slog.String("error", err.Error()))
				continue
			}
			logger.Info("scheduled control plane backup taken", slog.String("name", info.Name), slog.Int64("size_bytes", info.SizeBytes))
			if n, err := m.Prune(retain); err != nil {
				logger.Error("prune control plane backups failed", slog.String("error", err.Error()))
			} else if n > 0 {
				logger.Info("pruned old scheduled control plane backups", slog.Int("removed", n))
			}
		}
	}
}

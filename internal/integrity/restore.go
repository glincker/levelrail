package integrity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PendingFilename is the marker the next boot reads to swap in a staged snapshot.
const PendingFilename = "restore-pending.json"

// ErrNoVerifiedBackup means no snapshot passed verification.
var ErrNoVerifiedBackup = errors.New("no verified control plane snapshot is available")

// Pending describes a snapshot staged to replace the database at the next boot.
type Pending struct {
	Name          string    `json:"name"`
	SHA256        string    `json:"sha256"`
	SchemaVersion int       `json:"schema_version"`
	StagedAt      time.Time `json:"staged_at"`
	StagedBy      string    `json:"staged_by,omitempty"`
}

// Applied is the result of swapping a staged snapshot in at boot.
type Applied struct {
	Pending Pending `json:"pending"`
	Aside   string  `json:"aside"`
}

// BackupLister is the slice of cpbackup.Manager the restore path needs.
type BackupLister interface {
	List() ([]cpbackup.Info, error)
	Verify(ctx context.Context, name string) (cpbackup.VerifyResult, error)
}

// Candidate is one snapshot the operator can restore.
type Candidate struct {
	Name       string     `json:"name"`
	SizeBytes  int64      `json:"size_bytes"`
	CreatedAt  time.Time  `json:"created_at"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	Verified   bool       `json:"verified"`
}

func pendingPath(dataDir string) string { return filepath.Join(dataDir, PendingFilename) }

// ReadPending returns the staged restore, or nil when none is staged.
func ReadPending(dataDir string) (*Pending, error) {
	data, err := os.ReadFile(pendingPath(dataDir)) //nolint:gosec // fixed filename under the operator data dir
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending restore: %w", err)
	}
	var p Pending
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse pending restore: %w", err)
	}
	return &p, nil
}

// ClearPending cancels a staged restore.
func ClearPending(dataDir string) error {
	if err := os.Remove(pendingPath(dataDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear pending restore: %w", err)
	}
	return nil
}

func backupPath(dataDir, name string) string { return filepath.Join(dataDir, cpbackup.DirName, name) }

// Stage verifies the named snapshot and marks it to replace the database at the next boot.
func Stage(ctx context.Context, dataDir, name, by string, now time.Time) (Pending, error) {
	if !cpbackup.ValidName(name) {
		return Pending{}, cpbackup.ErrInvalidName
	}
	path := backupPath(dataDir, name)
	version, err := store.InspectSnapshot(ctx, path)
	if err != nil {
		return Pending{}, fmt.Errorf("snapshot rejected: %w", err)
	}
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		return Pending{}, fmt.Errorf("read supported schema: %w", err)
	}
	if version > latest {
		return Pending{}, fmt.Errorf("snapshot is at schema version %d, newer than the %d this binary supports", version, latest)
	}
	_, sum, err := store.FileSHA256(path)
	if err != nil {
		return Pending{}, fmt.Errorf("hash snapshot: %w", err)
	}
	p := Pending{Name: name, SHA256: sum, SchemaVersion: version, StagedAt: now.UTC().Truncate(time.Second), StagedBy: by}
	data, err := json.Marshal(p)
	if err != nil {
		return Pending{}, fmt.Errorf("encode pending restore: %w", err)
	}
	tmp := pendingPath(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return Pending{}, fmt.Errorf("write pending restore: %w", err)
	}
	if err := os.Rename(tmp, pendingPath(dataDir)); err != nil {
		_ = os.Remove(tmp)
		return Pending{}, fmt.Errorf("stage pending restore: %w", err)
	}
	return p, nil
}

// ApplyPending swaps a staged snapshot in. It runs before the store opens, so no connection holds the file. The marker is removed whether or not the swap succeeds, so a bad snapshot cannot loop every boot.
func ApplyPending(ctx context.Context, dataDir, dbFile string, now time.Time) (*Applied, error) {
	p, err := ReadPending(dataDir)
	if err != nil {
		_ = ClearPending(dataDir)
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	defer func() { _ = ClearPending(dataDir) }()
	if !cpbackup.ValidName(p.Name) {
		return nil, cpbackup.ErrInvalidName
	}
	src := backupPath(dataDir, p.Name)
	if _, sum, err := store.FileSHA256(src); err != nil || sum != p.SHA256 {
		return nil, fmt.Errorf("staged snapshot %s changed after it was verified, restore cancelled", p.Name)
	}
	aside, _, err := Swap(ctx, dataDir, dbFile, src, now)
	if err != nil {
		return nil, err
	}
	return &Applied{Pending: *p, Aside: aside}, nil
}

// Swap replaces the live database with the snapshot at src, keeping the old file beside it. The control plane must not be running.
func Swap(ctx context.Context, dataDir, dbFile, src string, now time.Time) (aside string, version int, err error) {
	version, err = store.InspectSnapshot(ctx, src)
	if err != nil {
		return "", 0, fmt.Errorf("backup file rejected: %w", err)
	}
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		return "", 0, fmt.Errorf("read supported schema: %w", err)
	}
	if version > latest {
		return "", 0, fmt.Errorf("backup is at schema version %d, newer than the %d this binary supports; use a newer release", version, latest)
	}
	live := filepath.Join(dataDir, dbFile)
	if _, statErr := os.Stat(live); statErr == nil {
		aside = live + ".before-restore-" + now.UTC().Format("20060102T150405Z")
		if err := os.Rename(live, aside); err != nil {
			return "", 0, fmt.Errorf("move current database aside: %w", err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(live + suffix); err != nil && !os.IsNotExist(err) {
			return "", 0, fmt.Errorf("remove stale %s file: %w", suffix, err)
		}
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled data dir
		return "", 0, fmt.Errorf("create data dir: %w", err)
	}
	if err := copySynced(src, live); err != nil {
		return "", 0, fmt.Errorf("put backup in place: %w", err)
	}
	return aside, version, nil
}

func copySynced(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // caller validated the snapshot path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := dst + ".restoring"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // operator-controlled data dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// Candidates lists snapshots newest first with their last recorded verification.
func Candidates(m BackupLister) ([]Candidate, error) {
	infos, err := m.List()
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	out := make([]Candidate, 0, len(infos))
	for _, in := range infos {
		out = append(out, Candidate{Name: in.Name, SizeBytes: in.SizeBytes, CreatedAt: in.CreatedAt, VerifiedAt: in.VerifiedAt, Verified: in.VerifiedOK != nil && *in.VerifiedOK})
	}
	return out, nil
}

// LatestVerified re-verifies snapshots newest first and returns the first that passes.
func LatestVerified(ctx context.Context, m BackupLister) (string, error) {
	infos, err := m.List()
	if err != nil {
		return "", fmt.Errorf("list snapshots: %w", err)
	}
	for _, in := range infos {
		res, err := m.Verify(ctx, in.Name)
		if err == nil && res.OK {
			return in.Name, nil
		}
	}
	return "", ErrNoVerifiedBackup
}

package selfupgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RestorePointDir is the directory under the data dir that holds restore points.
const RestorePointDir = "restore-points"

// Restore point file names and retention defaults.
const (
	rpMeta               = "meta.json"
	rpDB                 = "database.sqlite"
	rpBinary             = "binary"
	rpUnit               = "unit"
	DefaultRestorePoints = 3
	DefaultRestoreMaxMB  = 2048
)

// ErrNoRestorePoint means the id names no kept restore point.
var ErrNoRestorePoint = errors.New("restore point not found")

// RestorePoint is a consistent copy of the database, the binary and the unit
// file taken just before an upgrade.
type RestorePoint struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Schema    int       `json:"schema"`
	AttemptID string    `json:"attempt_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
	HasBinary bool      `json:"has_binary"`
	HasUnit   bool      `json:"has_unit"`
}

// RestorePoints keeps restore points as one directory each, so they stay
// readable while the database itself is being swapped.
type RestorePoints struct{ Dir string }

// NewRestorePoints returns the restore points under dataDir.
func NewRestorePoints(dataDir string) *RestorePoints {
	return &RestorePoints{Dir: filepath.Join(dataDir, RestorePointDir)}
}

func (s *RestorePoints) dir(id string) string { return filepath.Join(s.Dir, id) }

// DBPath locates a point's database copy.
func (s *RestorePoints) DBPath(id string) string { return filepath.Join(s.dir(id), rpDB) }

// BinaryPath locates a point's binary copy.
func (s *RestorePoints) BinaryPath(id string) string { return filepath.Join(s.dir(id), rpBinary) }

// UnitPath locates a point's unit file copy.
func (s *RestorePoints) UnitPath(id string) string { return filepath.Join(s.dir(id), rpUnit) }

// Create copies the database snapshot and, when the paths are set and exist,
// the binary and unit file into a new point described by meta. Files are 0600.
func (s *RestorePoints) Create(meta RestorePoint, snapshotPath, binaryPath, unitPath string) (RestorePoint, error) {
	if !validID(meta.ID) {
		return RestorePoint{}, fmt.Errorf("invalid restore point id %q", meta.ID)
	}
	if err := os.MkdirAll(s.dir(meta.ID), 0o700); err != nil {
		return RestorePoint{}, fmt.Errorf("create restore point directory: %w", err)
	}
	fail := func(err error) (RestorePoint, error) {
		_ = os.RemoveAll(s.dir(meta.ID))
		return RestorePoint{}, err
	}
	n, err := copyPrivate(snapshotPath, s.DBPath(meta.ID))
	if err != nil {
		return fail(fmt.Errorf("copy database snapshot: %w", err))
	}
	meta.SizeBytes = n
	meta.HasBinary, meta.HasUnit = false, false
	if size, ok, err := copyIfPresent(binaryPath, s.BinaryPath(meta.ID)); err != nil {
		return fail(fmt.Errorf("copy binary: %w", err))
	} else if ok {
		meta.SizeBytes += size
		meta.HasBinary = true
	}
	if size, ok, err := copyIfPresent(unitPath, s.UnitPath(meta.ID)); err != nil {
		return fail(fmt.Errorf("copy unit file: %w", err))
	} else if ok {
		meta.SizeBytes += size
		meta.HasUnit = true
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fail(fmt.Errorf("encode restore point: %w", err))
	}
	if err := os.WriteFile(filepath.Join(s.dir(meta.ID), rpMeta), raw, 0o600); err != nil {
		return fail(fmt.Errorf("write restore point metadata: %w", err))
	}
	return meta, nil
}

func copyIfPresent(src, dst string) (int64, bool, error) {
	if src == "" {
		return 0, false, nil
	}
	if _, err := os.Stat(src); err != nil {
		return 0, false, nil
	}
	n, err := copyPrivate(src, dst)
	return n, err == nil, err
}

// List returns every readable point, newest first.
func (s *RestorePoints) List() ([]RestorePoint, error) {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read restore point directory: %w", err)
	}
	var out []RestorePoint
	for _, e := range entries {
		if !e.IsDir() || !validID(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.Dir, e.Name(), rpMeta)) //nolint:gosec // entry of the restore point directory
		if err != nil {
			continue
		}
		var p RestorePoint
		if json.Unmarshal(raw, &p) != nil || p.ID != e.Name() {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.After(out[k].CreatedAt) })
	return out, nil
}

// Get returns the point with id.
func (s *RestorePoints) Get(id string) (RestorePoint, error) {
	if validID(id) {
		pts, err := s.List()
		if err != nil {
			return RestorePoint{}, err
		}
		for _, p := range pts {
			if p.ID == id {
				return p, nil
			}
		}
	}
	return RestorePoint{}, fmt.Errorf("%w: %s", ErrNoRestorePoint, id)
}

// Remove deletes a point's directory.
func (s *RestorePoints) Remove(id string) {
	if validID(id) {
		_ = os.RemoveAll(s.dir(id))
	}
}

// PrunePlan returns the ids to delete from pts (newest first) so at most
// maxCount points and maxBytes bytes remain. The newest point always stays,
// even when it alone exceeds maxBytes. A limit of zero or less is not applied.
func PrunePlan(pts []RestorePoint, maxCount int, maxBytes int64) []string {
	var drop []string
	var total int64
	dropping := false
	for i, p := range pts {
		total += p.SizeBytes
		if i > 0 && ((maxCount > 0 && i >= maxCount) || (maxBytes > 0 && total > maxBytes)) {
			dropping = true
		}
		if dropping {
			drop = append(drop, p.ID)
		}
	}
	return drop
}

// Prune applies PrunePlan to the stored points and returns the removed ids.
func (s *RestorePoints) Prune(maxCount int, maxBytes int64) ([]string, error) {
	pts, err := s.List()
	if err != nil {
		return nil, err
	}
	drop := PrunePlan(pts, maxCount, maxBytes)
	for _, id := range drop {
		s.Remove(id)
	}
	return drop, nil
}

func copyPrivate(src, dst string) (int64, error) {
	in, err := os.Open(src) //nolint:gosec // paths come from the upgrade's own data and install directories
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", filepath.Base(src), err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // inside the restore point directory
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", filepath.Base(dst), err)
	}
	n, err := io.Copy(out, in)
	if err != nil {
		_ = out.Close()
		return 0, fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return 0, fmt.Errorf("sync %s: %w", filepath.Base(dst), err)
	}
	return n, out.Close()
}

package backup

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// SqliteSnapshotter produces a consistent snapshot of a SQLite .db file
// living inside a named Docker volume, the sqlite-path counterpart of
// VolumeArchiver.
type SqliteSnapshotter interface {
	Snapshot(ctx context.Context, volumeName, relPath string) (io.ReadCloser, error)
}

// ContainerSqliteSnapshotter is the real SqliteSnapshotter: a helper
// container mounts the volume read-only and runs sqlite3's own
// ".backup" command, safe under the app's own concurrent writes since
// it goes through SQLite's file locking rather than a filesystem copy.
type ContainerSqliteSnapshotter struct {
	Runtime docker.Runtime
}

const sqliteSnapshotPath = "/tmp/snapshot.db"

// Snapshot implements SqliteSnapshotter.
func (s *ContainerSqliteSnapshotter) Snapshot(ctx context.Context, volumeName, relPath string) (io.ReadCloser, error) {
	if err := ValidateSqlitePath(relPath); err != nil {
		return nil, err
	}

	id, err := createVolumeHelper(ctx, s.Runtime, volumeName, "sqlitebackup", true)
	if err != nil {
		return nil, fmt.Errorf("backup: snapshot sqlite volume %q: %w", volumeName, err)
	}

	src := path.Join(volumeMountPath, relPath)
	// alpine:3.20 has no sqlite3 preinstalled; apk add is the one network
	// dependency this path has beyond pulling the image itself.
	cmd := []string{"sh", "-c", fmt.Sprintf(
		"apk add --no-cache sqlite >/dev/null && sqlite3 %q %q && cat %q",
		src, ".backup '"+sqliteSnapshotPath+"'", sqliteSnapshotPath,
	)}

	rc, err := s.Runtime.Exec(ctx, id, cmd)
	if err != nil {
		_ = s.Runtime.Remove(context.Background(), id, true)
		return nil, fmt.Errorf("backup: snapshot sqlite volume %q: %w", volumeName, err)
	}
	return &removeContainerOnClose{ReadCloser: rc, runtime: s.Runtime, containerID: id}, nil
}

// ValidateSqlitePath rejects an empty, absolute, or volume-escaping path.
func ValidateSqlitePath(relPath string) error {
	if strings.TrimSpace(relPath) == "" {
		return fmt.Errorf("backup: sqlite path is empty")
	}
	if path.IsAbs(relPath) {
		return fmt.Errorf("backup: sqlite path %q must be relative to the volume", relPath)
	}
	clean := path.Clean(relPath)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("backup: sqlite path %q escapes the volume", relPath)
	}
	return nil
}

package backup

import (
	"context"
	"fmt"
)

// MoveVolume pipes archiver's Archive stream directly into restorer's
// Restore: no S3 target or store.BackupHistory row needed, since the data
// only ever exists transiently in flight. Callers must construct archiver
// against the volume's current node and restorer against its destination,
// and keep dockerVolumeName identical on both ends (see internal/api's
// handleMoveAppWithVolumes).
func MoveVolume(ctx context.Context, archiver VolumeArchiver, restorer VolumeRestorer, dockerVolumeName string) error {
	rc, err := archiver.Archive(ctx, dockerVolumeName)
	if err != nil {
		return fmt.Errorf("backup: move volume %q: archive: %w", dockerVolumeName, err)
	}
	defer func() {
		_ = rc.Close()
	}()

	if err := restorer.Restore(ctx, dockerVolumeName, rc); err != nil {
		return fmt.Errorf("backup: move volume %q: restore: %w", dockerVolumeName, err)
	}
	return nil
}

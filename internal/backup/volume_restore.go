package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// VolumeRestorer untars a previously archived stream back into a named
// Docker volume, in place: it wipes every existing file the volume holds
// first, so this is always a full replace, never a merge, matching
// Restorer's own "the restore command exited zero" contract.
type VolumeRestorer interface {
	Restore(ctx context.Context, volumeName string, archive io.Reader) error
}

// ContainerVolumeRestorer is the real VolumeRestorer: it runs the
// wipe-then-extract command inside a short-lived helper container with
// volumeName mounted read-write.
type ContainerVolumeRestorer struct {
	Runtime docker.Runtime
}

// Restore implements VolumeRestorer.
func (r *ContainerVolumeRestorer) Restore(ctx context.Context, volumeName string, archive io.Reader) error {
	id, err := createVolumeHelper(ctx, r.Runtime, volumeName, "volrestore", false)
	if err != nil {
		return fmt.Errorf("backup: restore volume %q: %w", volumeName, err)
	}
	defer func() {
		_ = r.Runtime.Remove(context.Background(), id, true)
	}()

	rc, err := r.Runtime.ExecWithInput(ctx, id, volumeRestoreCmd, archive)
	if err != nil {
		return fmt.Errorf("backup: restore volume %q: %w", volumeName, err)
	}
	defer func() {
		_ = rc.Close()
	}()
	// The restore command writes no meaningful stdout of its own; this
	// only drains it so the exec session reaches EOF and a non-zero exit
	// surfaces as an error here, the same reasoning ContainerRestorer.
	// Restore's own doc comment (restore.go) gives for its own drain.
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("backup: restore volume %q: %w", volumeName, err)
	}
	return nil
}

// volumeRestoreCmd wipes volumeMountPath's entire existing contents,
// including dotfiles, before extracting the incoming tar stream over it:
// a full replace, matching postgresRestoreCmd/mysqlRestoreCmd's own
// "drop and recreate" framing in restore.go, applied to a filesystem
// instead of a schema. The three glob patterns (regular entries, single-
// dot-prefixed entries, double-dot-prefixed entries) are the standard
// POSIX-shell way to clear a directory's contents without also matching
// "." or ".." themselves; rm -f silently tolerates whichever pattern
// matches nothing.
var volumeRestoreCmd = []string{"sh", "-c", "cd " + volumeMountPath + " && rm -rf -- ..?* .[!.]* *; exec tar -xf - -C " + volumeMountPath}

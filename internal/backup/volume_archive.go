package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// VolumeArchiver produces a tar archive of a named Docker volume's
// contents, the app-volume counterpart of Dumper for a database engine.
// The returned ReadCloser's Close must be called by every caller once
// done, the same contract Dumper's own doc comment establishes.
type VolumeArchiver interface {
	Archive(ctx context.Context, volumeName string) (io.ReadCloser, error)
}

// ContainerVolumeArchiver is the real VolumeArchiver: it creates a
// helper container with volumeName mounted read-only, tars its contents
// to stdout, and removes the container once the caller has read the
// stream to completion (or given up partway through).
type ContainerVolumeArchiver struct {
	Runtime docker.Runtime
}

// Archive implements VolumeArchiver.
func (a *ContainerVolumeArchiver) Archive(ctx context.Context, volumeName string) (io.ReadCloser, error) {
	id, err := createVolumeHelper(ctx, a.Runtime, volumeName, "volbackup", true)
	if err != nil {
		return nil, fmt.Errorf("backup: archive volume %q: %w", volumeName, err)
	}

	rc, err := a.Runtime.Exec(ctx, id, volumeArchiveCmd)
	if err != nil {
		_ = a.Runtime.Remove(context.Background(), id, true)
		return nil, fmt.Errorf("backup: archive volume %q: %w", volumeName, err)
	}
	return &removeContainerOnClose{ReadCloser: rc, runtime: a.Runtime, containerID: id}, nil
}

// volumeArchiveCmd tars volumeMountPath's entire contents to stdout,
// uncompressed: the object this produces goes through the same
// S3-compatible upload path every database dump already does, so there
// is no local-disk reason to compress, and busybox tar (the only tar
// Alpine ships without adding a package) has inconsistent gzip support
// across builds, plain tar does not.
var volumeArchiveCmd = []string{"tar", "-cf", "-", "-C", volumeMountPath, "."}

// removeContainerOnClose wraps an exec stream's ReadCloser so Close also
// force-removes the helper container it came from: the container has no
// reason to exist past that point, and a caller of Archive has no other
// way to know its ID to clean it up itself.
type removeContainerOnClose struct {
	io.ReadCloser
	runtime     docker.Runtime
	containerID string
}

func (r *removeContainerOnClose) Close() error {
	err := r.ReadCloser.Close()
	// Cleanup runs against context.Background(), not whatever ctx Archive
	// was called with: the ordinary caller (Runner.runDumpAndUpload's own
	// defer) closes this the instant it's done reading, which is exactly
	// when its own ctx is most likely to already be near cancellation;
	// removing the helper container must not be skipped just because of
	// that timing.
	if rmErr := r.runtime.Remove(context.Background(), r.containerID, true); rmErr != nil && err == nil {
		err = rmErr
	}
	return err
}

package dockerssh

import "context"

// CopyMount streams one of a container's own mount destinations through the
// tunnel and unpacks it into dstDir with the extraction safeguards applied.
func (s *Session) CopyMount(ctx context.Context, containerID, mountPath, dstDir string, lim Limits) (ExtractStats, error) {
	rc, err := s.ContainerArchive(ctx, containerID, mountPath)
	if err != nil {
		return ExtractStats{}, err
	}
	defer func() { _ = rc.Close() }()
	return ExtractArchive(rc, dstDir, lim)
}

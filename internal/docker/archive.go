package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
)

// Archive copy errors callers can match.
var (
	ErrArchivePathNotFound = errors.New("docker: path not found in container")
	ErrArchiveTooLarge     = errors.New("docker: archive stream exceeds the size limit")
	ErrArchiveUnsupported  = errors.New("docker: this node's container runtime does not support archive copy")
)

// PathStat is the stat Docker returns beside a copy-out stream.
type PathStat struct {
	Name       string
	Size       int64
	Mode       os.FileMode
	LinkTarget string
}

// ArchiveRuntime is the tar copy half of a runtime, kept off Runtime so
// the narrow test fakes need no stub. maxBytes caps the tar stream (0 means
// no cap); exceeding it fails the stream with ErrArchiveTooLarge.
type ArchiveRuntime interface {
	// CopyFromContainer streams the tar of path (absolute, inside the container).
	CopyFromContainer(ctx context.Context, containerID, path string, maxBytes int64) (io.ReadCloser, PathStat, error)
	// CopyToContainer extracts the tar (optionally gzip) in content under dir.
	CopyToContainer(ctx context.Context, containerID, dir string, content io.Reader, maxBytes int64) error
}

// CopyFromContainer implements ArchiveRuntime over GET /containers/{id}/archive.
func (c *Client) CopyFromContainer(ctx context.Context, containerID, path string, maxBytes int64) (io.ReadCloser, PathStat, error) {
	rc, st, err := c.cli.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil, PathStat{}, fmt.Errorf("%w: %s", ErrArchivePathNotFound, path)
		}
		return nil, PathStat{}, fmt.Errorf("docker: copy %s from %s: %w", path, containerID, err)
	}
	return CapReadCloser(rc, maxBytes), pathStatFromAPI(st), nil
}

// CopyToContainer implements ArchiveRuntime over PUT /containers/{id}/archive.
func (c *Client) CopyToContainer(ctx context.Context, containerID, dir string, content io.Reader, maxBytes int64) error {
	body := CapReader(content, maxBytes)
	if err := c.cli.CopyToContainer(ctx, containerID, dir, body, container.CopyToContainerOptions{}); err != nil {
		if errors.Is(err, ErrArchiveTooLarge) {
			return ErrArchiveTooLarge
		}
		return fmt.Errorf("docker: copy into %s:%s: %w", containerID, dir, err)
	}
	return nil
}

func pathStatFromAPI(st container.PathStat) PathStat {
	return PathStat{Name: st.Name, Size: st.Size, Mode: st.Mode, LinkTarget: st.LinkTarget}
}

type capReader struct {
	r    io.Reader
	left int64
}

// CapReader fails with ErrArchiveTooLarge once more than limit bytes are read.
// limit <= 0 returns r unchanged.
func CapReader(r io.Reader, limit int64) io.Reader {
	if limit <= 0 {
		return r
	}
	return &capReader{r: r, left: limit}
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left < 0 {
		return 0, ErrArchiveTooLarge
	}
	if int64(len(p)) > c.left+1 {
		p = p[:c.left+1]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	if c.left < 0 {
		return n - 1, ErrArchiveTooLarge
	}
	return n, err
}

type capReadCloser struct {
	io.Reader
	io.Closer
}

// CapReadCloser is CapReader for a stream that also needs closing.
func CapReadCloser(rc io.ReadCloser, limit int64) io.ReadCloser {
	if limit <= 0 {
		return rc
	}
	return capReadCloser{Reader: CapReader(rc, limit), Closer: rc}
}

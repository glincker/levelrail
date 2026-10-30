package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// FileMount is one inline file placed in a container before it starts.
type FileMount struct {
	// Path is absolute inside the container.
	Path string
	// Mode is the permission bits (0644 when zero).
	Mode    uint32
	Content []byte
}

// writeFilesBeforeStart uses the archive API on the created, not yet
// started container. It needs no volume or host path, so it works the same
// on a remote node (the agent runs this same Create) and on Podman, and a
// recreate rewrites the files because they live in the container layer.
func (c *Client) writeFilesBeforeStart(ctx context.Context, id string, files []FileMount) error {
	archive, err := filesTar(files)
	if err != nil {
		return err
	}
	if err := c.cli.CopyToContainer(ctx, id, "/", archive, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("docker: write %d file(s) into container: %w", len(files), err)
	}
	return nil
}

func filesTar(files []FileMount) (io.Reader, error) {
	sorted := append([]FileMount(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range sorted {
		mode := int64(f.Mode)
		if mode == 0 {
			mode = 0o644
		}
		name := strings.TrimPrefix(path.Clean(f.Path), "/")
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(f.Content)), Typeflag: tar.TypeReg}); err != nil {
			return nil, fmt.Errorf("docker: tar header for %q: %w", f.Path, err)
		}
		if _, err := tw.Write(f.Content); err != nil {
			return nil, fmt.Errorf("docker: tar body for %q: %w", f.Path, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("docker: tar close: %w", err)
	}
	return &buf, nil
}

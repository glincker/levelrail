package build

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
)

// Environment override and default for the size cap on an extracted static build.
const (
	EnvStaticMaxMB     = "APP_STATIC_BUILD_MAX_MB"
	DefaultStaticMaxMB = 2048
)

// ErrExtractTooLarge is returned when an image directory exceeds the size cap.
var ErrExtractTooLarge = errors.New("build: extracted output exceeds the size limit")

// StaticMaxBytesFromEnv reads the extraction cap in bytes; zero means unlimited.
func StaticMaxBytesFromEnv(lookup func(string) (string, bool)) int64 {
	mb := DefaultStaticMaxMB
	if v, ok := lookup(EnvStaticMaxMB); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			mb = n
		}
	}
	return int64(mb) << 20
}

// ImageExtractor copies a directory out of a built image into a host
// directory, then deletes the image. It never starts the image.
type ImageExtractor struct {
	docker   *dockerclient.Client
	maxBytes int64
}

// NewImageExtractor wraps docker; maxBytes caps the extracted size (0 is unlimited).
func NewImageExtractor(docker *dockerclient.Client, maxBytes int64) *ImageExtractor {
	return &ImageExtractor{docker: docker, maxBytes: maxBytes}
}

// ExtractDir copies srcPath of image tag into destDir and removes the image.
func (e *ImageExtractor) ExtractDir(ctx context.Context, tag, srcPath, destDir string) error {
	defer e.removeImage(tag)
	created, err := e.docker.ContainerCreate(ctx, &container.Config{Image: tag, Cmd: []string{"/none"}}, nil, nil, nil, "")
	if err != nil {
		return fmt.Errorf("build: create extraction container: %w", err)
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
		defer cancel()
		_ = e.docker.ContainerRemove(rmCtx, created.ID, container.RemoveOptions{Force: true})
	}()
	rc, _, err := e.docker.CopyFromContainer(ctx, created.ID, path.Join(srcPath, "."))
	if err != nil {
		return fmt.Errorf("build: copy %s from image: %w", srcPath, err)
	}
	defer rc.Close() //nolint:errcheck // read side of a finished copy
	return UntarConfined(rc, destDir, e.maxBytes)
}

func (e *ImageExtractor) removeImage(tag string) {
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	_, _ = e.docker.ImageRemove(ctx, tag, image.RemoveOptions{PruneChildren: true})
}

// UntarConfined unpacks regular files and directories of r under destDir,
// skipping links and special files, and fails past maxBytes (0 is unlimited).
func UntarConfined(r io.Reader, destDir string, maxBytes int64) error {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("build: create %q: %w", destDir, err)
	}
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("build: open %q: %w", destDir, err)
	}
	defer root.Close() //nolint:errcheck // confinement handle only
	var total int64
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("build: read tar: %w", err)
		}
		name := filepath.FromSlash(path.Clean(hdr.Name))
		if name == "." || !filepath.IsLocal(name) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o750); err != nil {
				return fmt.Errorf("build: mkdir %q: %w", name, err)
			}
		case tar.TypeReg:
			total += hdr.Size
			if maxBytes > 0 && total > maxBytes {
				return ErrExtractTooLarge
			}
			if err := writeConfined(root, name, tr); err != nil {
				return err
			}
		}
	}
}

func writeConfined(root *os.Root, name string, r io.Reader) (err error) {
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("build: mkdir %q: %w", dir, err)
		}
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("build: create %q: %w", name, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("build: close %q: %w", name, cerr)
		}
	}()
	if _, err := io.Copy(f, r); err != nil { //nolint:gosec // size is bounded by the caller's cumulative cap
		return fmt.Errorf("build: write %q: %w", name, err)
	}
	return nil
}

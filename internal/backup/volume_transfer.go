package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

var (
	// ErrTransferTooLarge means a volume archive went past the configured size cap.
	ErrTransferTooLarge = errors.New("backup: volume transfer exceeds the size limit")
	// ErrVolumeNotEmpty means an import was aimed at a volume that already holds files.
	ErrVolumeNotEmpty = errors.New("backup: the volume is not empty")
	// ErrUnsafeArchive means an import archive holds an entry that could escape the volume.
	ErrUnsafeArchive = errors.New("backup: the archive holds an unsafe entry")
)

// TransferRuntime is the slice of docker.Runtime a volume transfer needs.
type TransferRuntime interface {
	Create(ctx context.Context, spec docker.ContainerSpec) (string, error)
	Start(ctx context.Context, id string) error
	Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error)
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
	Remove(ctx context.Context, id string, force bool) error
}

// VolumeTransfer moves a named volume's files out of or into Levelrail as a tar.gz.
type VolumeTransfer struct {
	Runtime TransferRuntime
}

// TransferStats counts what crossed the wire; names and sizes only, never contents.
type TransferStats struct {
	Entries int64 `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

// ExportStream is a tar.gz of a volume that removes its helper container on Close.
type ExportStream struct {
	rc        io.ReadCloser
	rt        TransferRuntime
	id        string
	remaining int64
	n         int64
}

// Read implements io.Reader and fails closed once the archive passes the size cap.
func (e *ExportStream) Read(p []byte) (int, error) {
	n, err := e.rc.Read(p)
	e.n += int64(n)
	if e.n > e.remaining {
		return n, ErrTransferTooLarge
	}
	return n, err
}

// Bytes is how many archive bytes were read so far.
func (e *ExportStream) Bytes() int64 { return e.n }

// Close ends the stream and removes the helper container.
func (e *ExportStream) Close() error {
	err := e.rc.Close()
	if rmErr := e.rt.Remove(context.Background(), e.id, true); rmErr != nil && err == nil {
		err = rmErr
	}
	return err
}

func (t *VolumeTransfer) helper(ctx context.Context, volume, prefix string, readOnly bool) (string, error) {
	suffix, err := randomHelperSuffix()
	if err != nil {
		return "", fmt.Errorf("generate helper container name: %w", err)
	}
	id, err := t.Runtime.Create(ctx, docker.ContainerSpec{
		Name:    prefix + "-" + suffix,
		Image:   volumeHelperImage,
		Command: []string{"sleep", "86400"},
		Volumes: []docker.VolumeMount{{Name: volume, ContainerPath: volumeMountPath, ReadOnly: readOnly}},
	})
	if err != nil {
		return "", fmt.Errorf("create helper container: %w", err)
	}
	if err := t.Runtime.Start(ctx, id); err != nil {
		_ = t.Runtime.Remove(context.Background(), id, true)
		return "", fmt.Errorf("start helper container: %w", err)
	}
	return id, nil
}

// Export streams the volume as tar.gz through a read-only helper container.
func (t *VolumeTransfer) Export(ctx context.Context, volume string, maxBytes int64) (*ExportStream, error) {
	id, err := t.helper(ctx, volume, "volexport", true)
	if err != nil {
		return nil, fmt.Errorf("backup: export volume %q: %w", volume, err)
	}
	rc, err := t.Runtime.Exec(ctx, id, []string{"tar", "-czf", "-", "-C", volumeMountPath, "."})
	if err != nil {
		_ = t.Runtime.Remove(context.Background(), id, true)
		return nil, fmt.Errorf("backup: export volume %q: %w", volume, err)
	}
	return &ExportStream{rc: rc, rt: t.Runtime, id: id, remaining: maxBytes}, nil
}

// ImportOptions tunes Import.
type ImportOptions struct {
	// MaxBytes caps the unpacked size, headers included.
	MaxBytes int64
	// Overwrite lets Import replace a volume that already holds files.
	Overwrite bool
}

// Import unpacks a tar.gz into a volume. The archive is re-written entry by entry
// so nothing unvalidated reaches the extractor.
func (t *VolumeTransfer) Import(ctx context.Context, volume string, archive io.Reader, opts ImportOptions) (TransferStats, error) {
	id, err := t.helper(ctx, volume, "volimport", false)
	if err != nil {
		return TransferStats{}, fmt.Errorf("backup: import into volume %q: %w", volume, err)
	}
	defer func() { _ = t.Runtime.Remove(context.Background(), id, true) }()

	out, err := t.execOutput(ctx, id, []string{"sh", "-c", "ls -A " + volumeMountPath + " | head -n 1"})
	if err != nil {
		return TransferStats{}, fmt.Errorf("backup: inspect volume %q: %w", volume, err)
	}
	populated := strings.TrimSpace(out) != ""
	if populated && !opts.Overwrite {
		return TransferStats{}, ErrVolumeNotEmpty
	}

	type result struct {
		stats TransferStats
		err   error
	}
	pr, pw := io.Pipe()
	ready := make(chan struct{}, 1)
	done := make(chan result, 1)
	go func() {
		st, serr := sanitizeArchive(archive, pw, opts.MaxBytes, func() { ready <- struct{}{} })
		_ = pw.CloseWithError(serr)
		done <- result{st, serr}
	}()

	// The first entry must validate before anything is wiped.
	select {
	case <-ready:
	case r := <-done:
		if r.err != nil {
			return TransferStats{}, r.err
		}
		done <- r
	case <-ctx.Done():
		_ = pr.CloseWithError(ctx.Err())
		return TransferStats{}, ctx.Err()
	}
	if populated {
		wipe := []string{"sh", "-c", "cd " + volumeMountPath + " && rm -rf -- ..?* .[!.]* *"}
		if _, err := t.execOutput(ctx, id, wipe); err != nil {
			_ = pr.CloseWithError(err)
			return TransferStats{}, fmt.Errorf("backup: clear volume %q: %w", volume, err)
		}
	}

	rc, execErr := t.Runtime.ExecWithInput(ctx, id, []string{"tar", "-xf", "-", "-C", volumeMountPath}, pr)
	if execErr == nil {
		_, execErr = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	if execErr != nil {
		_ = pr.CloseWithError(execErr)
	}
	r := <-done
	if r.err != nil {
		return TransferStats{}, r.err
	}
	if execErr != nil {
		return TransferStats{}, fmt.Errorf("backup: import into volume %q: %w", volume, execErr)
	}
	return r.stats, nil
}

func (t *VolumeTransfer) execOutput(ctx context.Context, id string, cmd []string) (string, error) {
	rc, err := t.Runtime.Exec(ctx, id, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, 4096))
	if err != nil {
		return "", err
	}
	_, err = io.Copy(io.Discard, rc)
	return string(b), err
}

const tarBlock = 512

func unsafeEntry(name, why string) error {
	return fmt.Errorf("%w: %q %s", ErrUnsafeArchive, name, why)
}

// sanitizeArchive copies a tar.gz to dst as a plain tar, refusing any entry that
// could land outside the volume. onFirst runs once the first entry passed.
func sanitizeArchive(src io.Reader, dst io.Writer, maxBytes int64, onFirst func()) (TransferStats, error) {
	var st TransferStats
	zr, err := gzip.NewReader(src)
	if err != nil {
		return st, fmt.Errorf("backup: the file is not a gzip archive: %w", err)
	}
	defer func() { _ = zr.Close() }()
	tr := tar.NewReader(zr)
	tw := tar.NewWriter(dst)
	symlinks := map[string]bool{}
	first := true
	signal := func() {
		if first {
			first = false
			onFirst()
		}
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return st, fmt.Errorf("backup: read archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name, err := cleanEntryName(hdr.Name)
		if err != nil {
			return st, err
		}
		if name == "." {
			continue
		}
		if err := checkEntry(hdr, name, symlinks); err != nil {
			return st, err
		}
		st.Entries++
		st.Bytes += tarBlock + hdr.Size
		if st.Bytes > maxBytes {
			return st, ErrTransferTooLarge
		}
		signal()
		out := *hdr
		out.Name = name
		out.Format = tar.FormatPAX
		// Setuid and setgid bits on files from an outside archive are never wanted.
		out.Mode &^= 0o6000
		if err := tw.WriteHeader(&out); err != nil {
			return st, fmt.Errorf("backup: write archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := io.CopyN(tw, tr, hdr.Size); err != nil { // size already counted against the cap
				return st, fmt.Errorf("backup: copy %q: %w", name, err)
			}
		}
	}
	signal()
	if err := tw.Close(); err != nil {
		return st, fmt.Errorf("backup: finish archive: %w", err)
	}
	return st, nil
}

// cleanEntryName rejects absolute names and any ".." element, then normalizes.
func cleanEntryName(raw string) (string, error) {
	if raw == "" || strings.HasPrefix(raw, "/") || strings.ContainsRune(raw, 0) || strings.Contains(raw, `\`) {
		return "", unsafeEntry(raw, "has an absolute or invalid path")
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", unsafeEntry(raw, "climbs out of the volume")
		}
	}
	return path.Clean(raw), nil
}

func checkEntry(hdr *tar.Header, name string, symlinks map[string]bool) error {
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeDir, tar.TypeSymlink, tar.TypeLink:
	default:
		return unsafeEntry(name, "is a device, pipe or other special file")
	}
	// A symlink seen earlier must never act as a parent directory of a later entry.
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		if symlinks[dir] {
			return unsafeEntry(name, "is written through a symbolic link")
		}
	}
	switch hdr.Typeflag {
	case tar.TypeSymlink:
		target := hdr.Linkname
		if target == "" || strings.HasPrefix(target, "/") || strings.ContainsRune(target, 0) {
			return unsafeEntry(name, "links to an absolute or empty target")
		}
		resolved := path.Join(path.Dir(name), target) //nolint:gosec // string comparison only, nothing is extracted here
		if resolved == ".." || strings.HasPrefix(resolved, "../") {
			return unsafeEntry(name, "links outside the volume")
		}
		symlinks[name] = true
	case tar.TypeLink:
		if _, err := cleanEntryName(hdr.Linkname); err != nil {
			return unsafeEntry(name, "hard links outside the volume")
		}
	}
	return nil
}

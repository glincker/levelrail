package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
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

// Helper container limits: it only runs tar and rm on one volume.
const (
	transferHelperMemory = 256 << 20
	transferHelperPids   = 64
)

// transferHelperSpec is the locked-down helper: no network, every capability
// dropped except what tar needs to keep ownership, no-new-privileges, a
// read-only root filesystem and memory and process limits.
func transferHelperSpec(name, volume string, readOnly bool) docker.ContainerSpec {
	caps := []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID"}
	if readOnly {
		caps = []string{"DAC_READ_SEARCH"}
	}
	return docker.ContainerSpec{
		Name:           name,
		Image:          volumeHelperImage,
		Command:        []string{"sleep", "86400"},
		Volumes:        []docker.VolumeMount{{Name: volume, ContainerPath: volumeMountPath, ReadOnly: readOnly}},
		NetworkMode:    "none",
		Hardened:       true,
		HardenedCapAdd: caps,
		PidsLimit:      transferHelperPids,
		Resources:      &docker.Resources{MemoryBytes: transferHelperMemory, SwapMemoryBytes: transferHelperMemory},
	}
}

func (t *VolumeTransfer) helper(ctx context.Context, volume, prefix string, readOnly bool) (string, error) {
	suffix, err := randomHelperSuffix()
	if err != nil {
		return "", fmt.Errorf("generate helper container name: %w", err)
	}
	id, err := t.Runtime.Create(ctx, transferHelperSpec(prefix+"-"+suffix, volume, readOnly))
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
	// SpoolDir holds the staged archive; empty uses the system temp dir.
	SpoolDir string
	// Limits bounds entry names; zero fields use the defaults.
	Limits EntryLimits
	// BeforeMount runs after the archive is staged and just before the volume
	// is mounted, so the caller can re-check that the owning app is still stopped.
	BeforeMount func(ctx context.Context) error
}

// Import unpacks a tar.gz into a volume. The whole archive is validated and
// re-written to a temp file before the volume is touched, so a rejected archive
// leaves the volume as it was.
func (t *VolumeTransfer) Import(ctx context.Context, volume string, archive io.Reader, opts ImportOptions) (TransferStats, error) {
	// Validate and rewrite the whole archive first, so a bad late entry cannot
	// leave a cleared volume behind and the volume is not mounted during upload.
	if opts.SpoolDir != "" {
		if err := os.MkdirAll(opts.SpoolDir, 0o700); err != nil {
			return TransferStats{}, fmt.Errorf("backup: stage import archive: %w", err)
		}
	}
	spool, err := os.CreateTemp(opts.SpoolDir, SpoolFilePrefix+"*.tar")
	if err != nil {
		return TransferStats{}, fmt.Errorf("backup: stage import archive: %w", err)
	}
	defer func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}()
	stats, err := sanitizeArchiveCtx(ctx, archive, spool, opts.MaxBytes, opts.Limits)
	if err != nil {
		return TransferStats{}, err
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return TransferStats{}, fmt.Errorf("backup: stage import archive: %w", err)
	}
	if opts.BeforeMount != nil {
		if err := opts.BeforeMount(ctx); err != nil {
			return TransferStats{}, err
		}
	}

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
	if populated {
		wipe := []string{"sh", "-c", "cd " + volumeMountPath + " && rm -rf -- ..?* .[!.]* *"}
		if _, err := t.execOutput(ctx, id, wipe); err != nil {
			return TransferStats{}, fmt.Errorf("backup: clear volume %q: %w", volume, err)
		}
	}

	rc, err := t.Runtime.ExecWithInput(ctx, id, []string{"tar", "-xf", "-", "-C", volumeMountPath}, spool)
	if err == nil {
		_, err = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	if err != nil {
		return TransferStats{}, fmt.Errorf("backup: import into volume %q: %w", volume, err)
	}
	return stats, nil
}

// SpoolFilePrefix names staged import archives so a startup sweep can find leftovers.
const SpoolFilePrefix = "levelrail-volimport-"

// CleanSpoolDir removes staged archives a crashed import left behind.
func CleanSpoolDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), SpoolFilePrefix) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
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
	if len(name) > 256 {
		name = name[:256] + "..."
	}
	return fmt.Errorf("%w: %q %s", ErrUnsafeArchive, name, why)
}

// archiveState tracks what earlier entries declared, so later ones are judged
// against the tree the extractor will actually build.
type archiveState struct {
	symlinks map[string]string // entry name to its link target
	regular  map[string]bool   // entry names written as plain files
}

// sanitizeArchive copies a tar.gz to dst as a plain tar, refusing any entry that
// could land outside the volume.
func sanitizeArchive(src io.Reader, dst io.Writer, maxBytes int64) (TransferStats, error) {
	return sanitizeArchiveCtx(context.Background(), src, dst, maxBytes, EntryLimits{})
}

// sanitizeArchiveCtx is sanitizeArchive with a context checked once per entry
// and explicit name limits.
func sanitizeArchiveCtx(ctx context.Context, src io.Reader, dst io.Writer, maxBytes int64, lim EntryLimits) (TransferStats, error) {
	lim = lim.withDefaults()
	var st TransferStats
	zr, err := gzip.NewReader(src)
	if err != nil {
		return st, fmt.Errorf("backup: the file is not a gzip archive: %w", err)
	}
	defer func() { _ = zr.Close() }()
	tr := tar.NewReader(zr)
	tw := tar.NewWriter(dst)
	state := &archiveState{symlinks: map[string]string{}, regular: map[string]bool{}}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return st, fmt.Errorf("backup: read archive: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return st, err
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if err := lim.check(hdr); err != nil {
			return st, err
		}
		name, err := cleanEntryName(hdr.Name)
		if err != nil {
			return st, err
		}
		if name == "." {
			continue
		}
		if err := checkEntry(hdr, name, state); err != nil {
			return st, err
		}
		st.Entries++
		st.Bytes += tarBlock + hdr.Size + int64(len(hdr.Name)+len(hdr.Linkname)) + paxBytes(hdr.PAXRecords)
		if st.Bytes > maxBytes {
			return st, ErrTransferTooLarge
		}
		// Only basic fields are copied, so extended attributes and vendor records never reach the volume.
		out := tar.Header{
			Typeflag: hdr.Typeflag, Name: name, Linkname: hdr.Linkname, Size: hdr.Size, Mode: hdr.Mode,
			Uid: hdr.Uid, Gid: hdr.Gid, Uname: hdr.Uname, Gname: hdr.Gname,
			ModTime: hdr.ModTime, AccessTime: hdr.AccessTime, ChangeTime: hdr.ChangeTime,
			Format: tar.FormatPAX,
		}
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
	// A link declared before the link it passes through only resolves once the
	// whole archive is known, so every symlink is judged again against the final tree.
	for name, target := range state.symlinks {
		if err := checkSymlinkTarget(name, target, state.symlinks); err != nil {
			return st, err
		}
	}
	if err := tw.Close(); err != nil {
		return st, fmt.Errorf("backup: finish archive: %w", err)
	}
	return st, nil
}

// Defaults for EntryLimits, used when a field is left at zero.
const (
	DefaultMaxEntryNameBytes = 4096
	DefaultMaxEntryDepth     = 128
)

// EntryLimits bounds one archive entry's name and link target.
type EntryLimits struct {
	// MaxNameBytes caps the byte length of a name or link target; 0 uses the default.
	MaxNameBytes int
	// MaxDepth caps the path components of a name or link target; 0 uses the default.
	MaxDepth int
}

func (l EntryLimits) withDefaults() EntryLimits {
	if l.MaxNameBytes <= 0 {
		l.MaxNameBytes = DefaultMaxEntryNameBytes
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = DefaultMaxEntryDepth
	}
	return l
}

// check rejects an oversized name or link target before any path parsing.
func (l EntryLimits) check(hdr *tar.Header) error {
	for _, p := range []string{hdr.Name, hdr.Linkname} {
		if len(p) > l.MaxNameBytes {
			return unsafeEntry(p, "has a name or link target that is too long")
		}
		if strings.Count(p, "/")+1 > l.MaxDepth {
			return unsafeEntry(p, "is nested too deeply")
		}
	}
	return nil
}

// paxBytes sizes extended header records so they count against the transfer cap.
func paxBytes(recs map[string]string) int64 {
	var n int64
	for k, v := range recs {
		n += int64(len(k) + len(v))
	}
	return n
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

func checkEntry(hdr *tar.Header, name string, st *archiveState) error {
	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeDir, tar.TypeSymlink, tar.TypeLink:
	default:
		return unsafeEntry(name, "is a device, pipe or other special file")
	}
	if err := checkParents(name, name, st.symlinks); err != nil {
		return err
	}
	delete(st.regular, name)
	switch hdr.Typeflag {
	case tar.TypeReg:
		st.regular[name] = true
	case tar.TypeSymlink:
		target := hdr.Linkname
		if target == "" || strings.HasPrefix(target, "/") || strings.ContainsRune(target, 0) {
			return unsafeEntry(name, "links to an absolute or empty target")
		}
		if err := checkSymlinkTarget(name, target, st.symlinks); err != nil {
			return err
		}
		st.symlinks[name] = target
	case tar.TypeLink:
		target, err := cleanEntryName(hdr.Linkname)
		if err != nil {
			return unsafeEntry(name, "hard links outside the volume")
		}
		if err := checkParents(name, target, st.symlinks); err != nil {
			return err
		}
		if !st.regular[target] {
			return unsafeEntry(name, "hard links to a file that is not earlier in the archive")
		}
	}
	return nil
}

// checkParents refuses a path whose parent directory is a symlink from the archive.
func checkParents(entry, p string, symlinks map[string]string) error {
	for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
		if _, ok := symlinks[dir]; ok {
			return unsafeEntry(entry, "is written through a symbolic link")
		}
	}
	return nil
}

// checkSymlinkTarget follows a symlink through the links known so far and
// refuses one that ends outside the volume or loops.
func checkSymlinkTarget(name, target string, symlinks map[string]string) error {
	var stack []string
	if dir := path.Dir(name); dir != "." {
		stack = strings.Split(dir, "/")
	}
	queue := strings.Split(target, "/")
	hops := 0
	for len(queue) > 0 {
		part := queue[0]
		queue = queue[1:]
		switch part {
		case "", ".":
		case "..":
			if len(stack) == 0 {
				return unsafeEntry(name, "links outside the volume")
			}
			stack = stack[:len(stack)-1]
		default:
			stack = append(stack, part)
			if next, ok := symlinks[strings.Join(stack, "/")]; ok {
				if hops++; hops > maxSymlinkHops {
					return unsafeEntry(name, "links through too many symbolic links")
				}
				stack = stack[:len(stack)-1]
				queue = append(strings.Split(next, "/"), queue...)
			}
		}
	}
	return nil
}

const maxSymlinkHops = 40

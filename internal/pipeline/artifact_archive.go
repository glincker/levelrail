package pipeline

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const (
	workspaceDir = "/workspace"
	// maxEntryNameBytes bounds one archive entry name or link target.
	maxEntryNameBytes = 4096
	// entryOverheadBytes is the tar header and padding allowance per entry
	// when sizing the raw stream cap.
	entryOverheadBytes = 4096
)

// cleanArtifactPath checks one declared workspace path and returns it
// cleaned and relative. It rejects absolute paths, parent traversal, and
// glob characters (paths are literal).
func cleanArtifactPath(p string) (string, error) {
	switch {
	case strings.TrimSpace(p) == "":
		return "", errors.New("artifact path is empty")
	case strings.ContainsAny(p, "\x00\n\r"):
		return "", fmt.Errorf("artifact path %q has a control character", p)
	case strings.ContainsAny(p, "*?["):
		return "", fmt.Errorf("artifact path %q: globs are not supported, list files or directories", p)
	case strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~"):
		return "", fmt.Errorf("artifact path %q must be relative to the workspace", p)
	}
	c := path.Clean(p)
	if c == "." {
		return "", fmt.Errorf("artifact path %q is the workspace root, list files or directories", p)
	}
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("artifact path %q leaves the workspace", p)
	}
	return c, nil
}

func cleanArtifactPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		c, err := cleanArtifactPath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// packLimits bound what one packed artifact may hold.
type packLimits struct {
	MaxUnpacked int64
	MaxEntries  int
	Secrets     []string
}

// streamCap sizes the raw tar cap handed to the runtime: the unpacked
// limit plus header overhead for the entry cap. Zero means no cap.
func (l packLimits) streamCap() int64 {
	if l.MaxUnpacked <= 0 {
		return 0
	}
	return l.MaxUnpacked + int64(max(l.MaxEntries, 1))*entryOverheadBytes
}

// packer rewrites the tar streams Docker returns for each declared path
// into one validated tar: names rebased under their workspace location,
// permissions normalised, and every entry checked before it is written.
type packer struct {
	tw       *tar.Writer
	lim      packLimits
	scan     *needleWriter
	entries  int
	unpacked int64
	symlinks map[string]bool
}

func newPacker(tw *tar.Writer, lim packLimits) *packer {
	return &packer{tw: tw, lim: lim, scan: newNeedleWriter(lim.Secrets), symlinks: map[string]bool{}}
}

// entryName rebases a name from Docker's archive of decl onto the
// workspace-relative path. Docker names the top entry after the last
// element of decl, so anything else is not part of what was asked for.
func entryName(decl, raw string) (string, error) {
	n := path.Clean(strings.TrimPrefix(raw, "./"))
	if n == "." || n == "" {
		return "", nil
	}
	if strings.ContainsRune(raw, 0) || len(raw) > maxEntryNameBytes {
		return "", fmt.Errorf("archive entry name %q is invalid", truncate(raw, 80))
	}
	if strings.HasPrefix(n, "/") || n == ".." || strings.HasPrefix(n, "../") {
		return "", fmt.Errorf("archive entry %q leaves the workspace", truncate(raw, 80))
	}
	base := path.Base(decl)
	if n != base && !strings.HasPrefix(n, base+"/") {
		return "", fmt.Errorf("archive entry %q is outside the requested path %q", truncate(raw, 80), decl)
	}
	return path.Join(path.Dir(decl), n), nil
}

// add copies one declared path's archive into the packed tar.
func (p *packer) add(decl string, src io.Reader, stat os.FileMode, statLink string) error {
	if stat&(os.ModeDevice|os.ModeCharDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeIrregular) != 0 {
		return fmt.Errorf("artifact path %q is a device, pipe or socket", decl)
	}
	if stat&os.ModeSymlink != 0 {
		if err := checkLinkTarget(path.Dir(decl)+"/x", statLink, "symlink"); err != nil {
			return fmt.Errorf("artifact path %q: %w", decl, err)
		}
	}
	tr := tar.NewReader(src)
	seen := false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive of %q: %w", decl, err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name, err := entryName(decl, h.Name)
		if err != nil {
			return err
		}
		if name == "" {
			continue
		}
		seen = true
		if err := p.addEntry(decl, name, h, tr); err != nil {
			return err
		}
	}
	if !seen {
		return fmt.Errorf("archive of %q was empty", decl)
	}
	return nil
}

func (p *packer) addEntry(decl, name string, h *tar.Header, body io.Reader) error {
	p.entries++
	if p.lim.MaxEntries > 0 && p.entries > p.lim.MaxEntries {
		return fmt.Errorf("artifact holds more than %d files", p.lim.MaxEntries)
	}
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if p.symlinks[parent] {
			return fmt.Errorf("archive entry %q is written through a symlink", name)
		}
	}
	out := &tar.Header{Name: name, ModTime: h.ModTime, Uid: h.Uid, Gid: h.Gid, Mode: h.Mode & 0o777}
	switch h.Typeflag {
	case tar.TypeDir:
		out.Typeflag = tar.TypeDir
		out.Name = name + "/"
	case tar.TypeReg:
		p.unpacked += h.Size
		if p.lim.MaxUnpacked > 0 && p.unpacked > p.lim.MaxUnpacked {
			return fmt.Errorf("archive expands past the %d byte limit", p.lim.MaxUnpacked)
		}
		out.Typeflag, out.Size = tar.TypeReg, h.Size
	case tar.TypeSymlink:
		if err := checkLinkTarget(name, h.Linkname, "symlink"); err != nil {
			return err
		}
		p.symlinks[name] = true
		out.Typeflag, out.Linkname = tar.TypeSymlink, h.Linkname
	case tar.TypeLink:
		target, err := entryName(decl, h.Linkname)
		if err != nil || target == "" {
			return fmt.Errorf("archive hard link %q points outside the requested path", name)
		}
		out.Typeflag, out.Linkname = tar.TypeLink, target
	default:
		return fmt.Errorf("archive entry %q has an unsupported type", name)
	}
	if err := p.tw.WriteHeader(out); err != nil {
		return err
	}
	if out.Typeflag != tar.TypeReg {
		return nil
	}
	p.scan.reset(name)
	if _, err := io.Copy(io.MultiWriter(p.tw, p.scan), body); err != nil {
		return err
	}
	return nil
}

// checkLinkTarget refuses a link target that is absolute or resolves
// outside the workspace, judged lexically from the link's own location.
func checkLinkTarget(linkPath, target, kind string) error {
	if len(target) > maxEntryNameBytes || strings.ContainsRune(target, 0) {
		return fmt.Errorf("archive %s %q has an invalid target", kind, linkPath)
	}
	resolved := path.Clean(path.Join(path.Dir(linkPath), target)) //nolint:gosec // lexical check of a header; nothing is extracted here
	if path.IsAbs(target) || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("archive %s %q points outside the workspace", kind, linkPath)
	}
	return nil
}

// errSecretInArtifact is what the scan reports when a file holds a secret value.
type errSecretInArtifact struct{ entry string }

func (e errSecretInArtifact) Error() string {
	return fmt.Sprintf("archive entry %q contains a secret value", e.entry)
}

// needleWriter flags any secret value in the bytes written through it,
// keeping an overlap between writes so a match split across chunks is found.
type needleWriter struct {
	needles [][]byte
	keep    int
	tail    []byte
	entry   string
}

func newNeedleWriter(secrets []string) *needleWriter {
	nw := &needleWriter{}
	for _, s := range secrets {
		if s == "" {
			continue
		}
		nw.needles = append(nw.needles, []byte(s))
		nw.keep = max(nw.keep, len(s))
	}
	return nw
}

func (n *needleWriter) reset(entry string) {
	n.entry, n.tail = entry, n.tail[:0]
}

func (n *needleWriter) Write(b []byte) (int, error) {
	if len(n.needles) == 0 {
		return len(b), nil
	}
	buf := append(n.tail, b...)
	for _, nd := range n.needles {
		if bytes.Contains(buf, nd) {
			return 0, errSecretInArtifact{entry: n.entry}
		}
	}
	if keep := n.keep - 1; len(buf) > keep {
		buf = buf[len(buf)-keep:]
	}
	n.tail = append(n.tail[:0], buf...)
	return len(b), nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

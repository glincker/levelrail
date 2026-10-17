package pipeline

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// Markers the in-container collect script prints around the base64 archive,
// so stray stderr text can never be mistaken for archive data.
const (
	collectBegin = "LRART-BEGIN"
	collectEnd   = "LRART-END"
	collectErr   = "LRART-ERR "
	restoreData  = "LRDATA"
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

// collectScript archives the declared paths inside the job container. The
// realpath check catches a path that is itself a symlink out of /workspace.
func collectScript(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = shellQuote("./" + p)
	}
	list := strings.Join(quoted, " ")
	var sb strings.Builder
	sb.WriteString("cd /workspace || exit 3\n")
	fmt.Fprintf(&sb, "for p in %s; do\n", list)
	sb.WriteString(`  if [ ! -e "$p" ] && [ ! -L "$p" ]; then echo "` + collectErr + `path not found: $p"; exit 3; fi
  r=$(realpath -- "$p" 2>/dev/null) || { echo "` + collectErr + `cannot resolve: $p"; exit 3; }
  case "$r" in /workspace|/workspace/*) ;; *) echo "` + collectErr + `path leaves the workspace: $p"; exit 3;; esac
done
`)
	fmt.Fprintf(&sb, "t=/tmp/lr-art.$$\ntar -czf \"$t\" -- %s 2>/dev/null || { echo \"%star failed\"; rm -f \"$t\"; exit 4; }\n", list, collectErr)
	fmt.Fprintf(&sb, "echo %s\nbase64 \"$t\"\necho %s\nrm -f \"$t\"\n", collectBegin, collectEnd)
	return sb.String()
}

// readCollected decodes the marked base64 block of the collect output into
// w. Text outside the markers is returned as diagnostics for error messages.
func readCollected(rc io.Reader, w io.Writer) (diag []string, err error) {
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	in, done := false, false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == collectBegin:
			in = true
		case line == collectEnd:
			in, done = false, true
		case in:
			raw, derr := base64.StdEncoding.DecodeString(line)
			if derr != nil {
				return diag, fmt.Errorf("archive stream is not valid base64: %w", derr)
			}
			if _, werr := w.Write(raw); werr != nil {
				return diag, werr
			}
		default:
			if len(diag) < 20 && line != "" {
				diag = append(diag, strings.TrimPrefix(truncate(line, 200), collectErr))
			}
		}
	}
	if err := sc.Err(); err != nil {
		return diag, err
	}
	if !done {
		return diag, errors.New("archive stream ended early")
	}
	return diag, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// restoreStream feeds a stored archive to `tar -x` inside the container as
// a heredoc on the script's own stdin, wrapped to 76 columns.
func restoreStream(archive io.Reader) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		header := "cd /workspace || exit 3\nbase64 -d <<'" + restoreData + "' | tar -xzf - -C /workspace || { echo '" + collectErr + "extract failed'; exit 4; }\n"
		if _, err := io.WriteString(pw, header); err != nil {
			return
		}
		buf := make([]byte, 57*64)
		for {
			n, err := io.ReadFull(archive, buf)
			for off := 0; off < n; off += 57 {
				end := min(off+57, n)
				line := base64.StdEncoding.EncodeToString(buf[off:end]) + "\n"
				if _, werr := io.WriteString(pw, line); werr != nil {
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					_ = pw.CloseWithError(err)
					return
				}
				break
			}
		}
		_, _ = io.WriteString(pw, restoreData+"\n")
		_ = pw.Close()
	}()
	return pr
}

// validateArchive walks a gzip tar and refuses anything that could write
// or link outside the workspace on extraction, anything bigger than
// maxUnpacked once expanded, and any file containing a secret value.
func validateArchive(r io.Reader, secrets []string, maxUnpacked int64) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("artifact is not a gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	symlinks := map[string]bool{}
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("archive entry %q leaves the workspace", h.Name)
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if symlinks[parent] {
				return fmt.Errorf("archive entry %q is written through a symlink", h.Name)
			}
		}
		switch h.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg:
			total += h.Size
			if maxUnpacked > 0 && total > maxUnpacked {
				return fmt.Errorf("archive expands past the %d byte limit", maxUnpacked)
			}
			if found, err := containsAny(tr, secrets); err != nil {
				return fmt.Errorf("scan %q: %w", h.Name, err)
			} else if found {
				return fmt.Errorf("archive entry %q contains a secret value", h.Name)
			}
		case tar.TypeSymlink:
			symlinks[name] = true
			target := h.Linkname
			resolved := path.Clean(path.Join(path.Dir(name), target)) //nolint:gosec // lexical check of a header; nothing is extracted here
			if path.IsAbs(target) || resolved == ".." || strings.HasPrefix(resolved, "../") {
				return fmt.Errorf("archive symlink %q points outside the workspace", h.Name)
			}
		case tar.TypeLink:
			target := path.Clean(strings.TrimPrefix(h.Linkname, "./"))
			if path.IsAbs(h.Linkname) || target == ".." || strings.HasPrefix(target, "../") {
				return fmt.Errorf("archive hard link %q points outside the workspace", h.Name)
			}
		default:
			return fmt.Errorf("archive entry %q has an unsupported type", h.Name)
		}
	}
}

// containsAny reports whether the stream holds any needle, scanning in
// chunks with an overlap so a match split across chunks is still found.
func containsAny(r io.Reader, needles []string) (bool, error) {
	var longest int
	var bs [][]byte
	for _, n := range needles {
		if n == "" {
			continue
		}
		bs = append(bs, []byte(n))
		longest = max(longest, len(n))
	}
	if len(bs) == 0 {
		_, err := io.Copy(io.Discard, r)
		return false, err
	}
	buf := make([]byte, 0, 64*1024+longest)
	chunk := make([]byte, 64*1024)
	for {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		for _, b := range bs {
			if bytes.Contains(buf, b) {
				return true, nil
			}
		}
		if len(buf) > longest {
			buf = append(buf[:0], buf[len(buf)-(longest-1):]...)
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

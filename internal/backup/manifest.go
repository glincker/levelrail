package backup

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// TreeManifest summarizes a volume archive's content: counts plus an
// order-independent digest over every entry's path, type, mode, owner, size
// and content hash. Two archives of the same tree produce the same digest
// whatever order the tar tool walked it in.
type TreeManifest struct {
	Files  int64
	Dirs   int64
	Bytes  int64
	Digest string
}

// ComputeManifest reads a tar stream to its end. The volume root entry (".")
// is ignored because extraction legitimately rewrites its mode and times.
func ComputeManifest(r io.Reader) (TreeManifest, error) {
	tr := tar.NewReader(r)
	var m TreeManifest
	var lines []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return TreeManifest{}, fmt.Errorf("read tar entry: %w", err)
		}
		name := strings.TrimPrefix(strings.TrimPrefix(hdr.Name, "./"), "/")
		name = strings.TrimSuffix(name, "/")
		if name == "" || name == "." {
			continue
		}
		var content string
		switch hdr.Typeflag {
		case tar.TypeReg:
			h := sha256.New()
			n, cerr := io.Copy(h, io.LimitReader(tr, hdr.Size))
			if cerr != nil {
				return TreeManifest{}, fmt.Errorf("read %q: %w", name, cerr)
			}
			content = hex.EncodeToString(h.Sum(nil))
			m.Files++
			m.Bytes += n
		case tar.TypeDir:
			m.Dirs++
		case tar.TypeSymlink, tar.TypeLink:
			content = hdr.Linkname
			m.Files++
		default:
			m.Files++
		}
		typ := string(rune(hdr.Typeflag))
		if hdr.Typeflag == tar.TypeDir {
			typ = "d"
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%o\x00%d\x00%d\x00%s", name, typ, hdr.Mode&0o7777, hdr.Uid, hdr.Gid, content)))
		lines = append(lines, hex.EncodeToString(sum[:]))
	}
	sort.Strings(lines)
	all := sha256.New()
	for _, l := range lines {
		_, _ = all.Write([]byte(l))
	}
	m.Digest = hex.EncodeToString(all.Sum(nil))
	return m, nil
}

// manifestTee returns a reader that yields src unchanged while a goroutine
// computes src's TreeManifest. wait must be called after the returned reader
// has been fully consumed or abandoned.
func manifestTee(src io.Reader) (io.Reader, func() (TreeManifest, error)) {
	pr, pw := io.Pipe()
	done := make(chan struct{})
	var m TreeManifest
	var merr error
	go func() {
		defer close(done)
		m, merr = ComputeManifest(pr)
		_, _ = io.Copy(io.Discard, pr)
	}()
	tee := io.TeeReader(src, pw)
	return &teeCloser{Reader: tee, pw: pw}, func() (TreeManifest, error) {
		_ = pw.Close()
		<-done
		return m, merr
	}
}

type teeCloser struct {
	io.Reader
	pw *io.PipeWriter
}

// Read closes the manifest pipe on EOF or error so the parser can finish.
func (t *teeCloser) Read(p []byte) (int, error) {
	n, err := t.Reader.Read(p)
	if err != nil {
		_ = t.pw.Close()
	}
	return n, err
}

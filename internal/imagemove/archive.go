package imagemove

import (
	"archive/tar"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	maxJSONEntryBytes = 4 << 20
	maxJSONTotalBytes = 32 << 20
	maxIndexDepth     = 3
)

// ErrArchiveContent is returned when the saved archive carries no readable
// image config, so its content cannot be verified.
var ErrArchiveContent = errors.New("the image archive has no readable config")

// ArchiveInfo is what a streamed `docker save` archive says about its image.
type ArchiveInfo struct {
	// ConfigDigest is the sha256 of the image config blob, which is the
	// image ID on a classic (overlay2) image store.
	ConfigDigest string
	DiffIDs      []string
}

// scanArchive reads a docker save tar to its end and extracts the image
// config digest and rootfs diff IDs. Memory is bounded: only small entries
// that start like JSON are kept, and nothing is ever written to disk.
func scanArchive(r io.Reader, ref string) (ArchiveInfo, error) {
	files := map[string][]byte{}
	var kept int64
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ArchiveInfo{}, fmt.Errorf("read the image archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size <= 0 || hdr.Size > maxJSONEntryBytes || kept+hdr.Size > maxJSONTotalBytes {
			continue
		}
		br := bufio.NewReader(tr)
		first, err := br.Peek(1)
		if err != nil || (first[0] != '{' && first[0] != '[') {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(br, hdr.Size))
		if err != nil {
			return ArchiveInfo{}, fmt.Errorf("read the image archive: %w", err)
		}
		kept += int64(len(body))
		files[cleanEntryName(hdr.Name)] = body
	}
	return resolveArchive(files, ref)
}

func cleanEntryName(name string) string {
	return strings.TrimPrefix(path.Clean(strings.TrimPrefix(name, "./")), "/")
}

func resolveArchive(files map[string][]byte, ref string) (ArchiveInfo, error) {
	cfgPath, err := configPath(files, ref)
	if err != nil {
		return ArchiveInfo{}, err
	}
	cfg, ok := files[cfgPath]
	if !ok {
		return ArchiveInfo{}, fmt.Errorf("%w: %s is missing from the archive", ErrArchiveContent, cfgPath)
	}
	var parsed struct {
		RootFS struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		return ArchiveInfo{}, fmt.Errorf("%w: %s is not valid JSON", ErrArchiveContent, cfgPath)
	}
	if len(parsed.RootFS.DiffIDs) == 0 {
		return ArchiveInfo{}, fmt.Errorf("%w: %s lists no rootfs diff IDs", ErrArchiveContent, cfgPath)
	}
	sum := sha256.Sum256(cfg)
	return ArchiveInfo{ConfigDigest: "sha256:" + hex.EncodeToString(sum[:]), DiffIDs: parsed.RootFS.DiffIDs}, nil
}

// configPath finds the config blob's path from manifest.json (Docker
// layout) or, failing that, index.json (OCI layout).
func configPath(files map[string][]byte, ref string) (string, error) {
	if raw, ok := files["manifest.json"]; ok {
		var list []struct {
			Config   string   `json:"Config"`
			RepoTags []string `json:"RepoTags"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return "", fmt.Errorf("%w: manifest.json is not valid", ErrArchiveContent)
		}
		pick := ""
		for _, e := range list {
			if e.Config == "" {
				continue
			}
			if pick == "" {
				pick = e.Config
			}
			for _, tag := range e.RepoTags {
				if tag == ref {
					return cleanEntryName(e.Config), nil
				}
			}
		}
		if pick != "" {
			return cleanEntryName(pick), nil
		}
	}
	if raw, ok := files["index.json"]; ok {
		if p, ok := ociConfigPath(files, raw, 0); ok {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: no manifest.json or index.json names a config", ErrArchiveContent)
}

func blobPath(digest string) (string, bool) {
	algo, sum, ok := strings.Cut(digest, ":")
	if !ok || algo != "sha256" || sum == "" || strings.ContainsAny(sum, "/.") {
		return "", false
	}
	return "blobs/sha256/" + sum, true
}

func ociConfigPath(files map[string][]byte, raw []byte, depth int) (string, bool) {
	if depth >= maxIndexDepth {
		return "", false
	}
	var doc struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return "", false
	}
	if doc.Config.Digest != "" {
		return blobPath(doc.Config.Digest)
	}
	for _, m := range doc.Manifests {
		p, ok := blobPath(m.Digest)
		if !ok {
			continue
		}
		child, ok := files[p]
		if !ok {
			continue
		}
		if cfg, ok := ociConfigPath(files, child, depth+1); ok {
			return cfg, true
		}
	}
	return "", false
}

// teeScan copies src to r while a goroutine scans the same bytes, so the
// load is never held up and the archive is never buffered whole.
type teeScan struct {
	pw   *io.PipeWriter
	done chan struct{}
	info ArchiveInfo
	err  error
	r    io.Reader
}

func startScan(src io.Reader, ref string) *teeScan {
	pr, pw := io.Pipe()
	s := &teeScan{pw: pw, done: make(chan struct{})}
	s.r = io.TeeReader(src, pw)
	go func() {
		defer close(s.done)
		s.info, s.err = scanArchive(pr, ref)
		_, _ = io.Copy(io.Discard, pr)
	}()
	return s
}

func (s *teeScan) finish() (ArchiveInfo, error) {
	_ = s.pw.Close()
	<-s.done
	return s.info, s.err
}

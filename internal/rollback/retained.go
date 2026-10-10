// Package rollback plans and applies a return to an earlier control plane
// release. Applying is host-side only: the control plane process never
// replaces its own binary (see adr/028-release-rollback.md).
package rollback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// KeepCount is how many release binaries stay on disk for instant, offline
// rollback.
const KeepCount = 3

const (
	sumSuffix  = ".sha256"
	metaSuffix = ".json"
	dirSuffix  = ".releases"
)

// ErrInvalidVersion means a version string is not a release tag.
var ErrInvalidVersion = errors.New("not a release version")

var versionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+([-+][0-9A-Za-z.\-]+)?$`)

// ValidVersion reports whether v looks like a release tag. It also keeps tags
// safe to use inside a file name.
func ValidVersion(v string) bool { return versionPattern.MatchString(v) }

// Retained is one release binary kept on this host.
type Retained struct {
	Version       string    `json:"version"`
	Path          string    `json:"-"`
	SHA256        string    `json:"sha256"`
	SizeBytes     int64     `json:"size_bytes"`
	SchemaVersion int       `json:"schema_version"`
	RetainedAt    time.Time `json:"retained_at"`
}

type metaFile struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
}

// Dir returns the directory that holds retained binaries for the executable at exe.
func Dir(exe string) string { return exe + dirSuffix }

func binaryFile(dir, name, version string) string {
	return filepath.Join(dir, name+"-"+version)
}

// FileSHA256 returns the hex SHA-256 and size of the file at path.
func FileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path) //nolint:gosec // operator-controlled releases dir
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Retain copies src into dir as the binary for version, records its SHA-256
// and schema version beside it, then prunes to KeepCount, never removing
// version itself. schemaVersion is -1 when unknown.
func Retain(dir, name, version, src string, schemaVersion int, now time.Time) (Retained, error) {
	if !ValidVersion(version) {
		return Retained{}, fmt.Errorf("%w: %q", ErrInvalidVersion, version)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // holds executables, root-owned and world-readable like the installed binary
		return Retained{}, fmt.Errorf("create releases dir: %w", err)
	}
	dst := binaryFile(dir, name, version)
	if err := copyExecutable(src, dst); err != nil {
		return Retained{}, fmt.Errorf("retain %s: %w", version, err)
	}
	sum, size, err := FileSHA256(dst)
	if err != nil {
		return Retained{}, err
	}
	if err := os.WriteFile(dst+sumSuffix, []byte(sum+"\n"), 0o644); err != nil { //nolint:gosec // checksum sidecar, not a secret
		return Retained{}, fmt.Errorf("write checksum for %s: %w", version, err)
	}
	meta, err := json.Marshal(metaFile{Version: version, SchemaVersion: schemaVersion})
	if err != nil {
		return Retained{}, fmt.Errorf("encode metadata: %w", err)
	}
	if err := os.WriteFile(dst+metaSuffix, meta, 0o644); err != nil { //nolint:gosec // metadata sidecar, not a secret
		return Retained{}, fmt.Errorf("write metadata for %s: %w", version, err)
	}
	if err := prune(dir, name, version); err != nil {
		return Retained{}, err
	}
	return Retained{Version: version, Path: dst, SHA256: sum, SizeBytes: size, SchemaVersion: schemaVersion, RetainedAt: now}, nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // operator-supplied path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gosec // executable copy, same mode as the installed binary
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// List returns retained binaries newest first, with the SHA-256 recorded when
// each was retained (not recomputed: Verify does that before use).
func List(dir, name string) ([]Retained, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Retained{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read releases dir: %w", err)
	}
	prefix := name + "-"
	var out []Retained
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || strings.HasSuffix(n, sumSuffix) || strings.HasSuffix(n, metaSuffix) || strings.HasSuffix(n, ".tmp") {
			continue
		}
		version := strings.TrimPrefix(n, prefix)
		if !ValidVersion(version) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		r := Retained{Version: version, Path: filepath.Join(dir, n), SizeBytes: info.Size(), SchemaVersion: -1, RetainedAt: info.ModTime().UTC()}
		if raw, err := os.ReadFile(r.Path + sumSuffix); err == nil { //nolint:gosec // path built from a validated version
			r.SHA256 = strings.TrimSpace(string(raw))
		}
		if raw, err := os.ReadFile(r.Path + metaSuffix); err == nil { //nolint:gosec // path built from a validated version
			var m metaFile
			if json.Unmarshal(raw, &m) == nil && m.SchemaVersion >= 0 {
				r.SchemaVersion = m.SchemaVersion
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RetainedAt.After(out[j].RetainedAt) })
	return out, nil
}

// Find returns the retained binary for version.
func Find(dir, name, version string) (Retained, bool, error) {
	list, err := List(dir, name)
	if err != nil {
		return Retained{}, false, err
	}
	for _, r := range list {
		if r.Version == version {
			return r, true, nil
		}
	}
	return Retained{}, false, nil
}

// ErrChecksum means a retained binary no longer matches its recorded SHA-256.
var ErrChecksum = errors.New("retained binary does not match its recorded checksum")

// Verify recomputes r's SHA-256 and compares it with the recorded one. A
// binary with no recorded checksum is refused: it was not retained by the
// verified install path.
func Verify(r Retained) error {
	if r.SHA256 == "" {
		return fmt.Errorf("%w: no checksum recorded for %s", ErrChecksum, r.Version)
	}
	sum, _, err := FileSHA256(r.Path)
	if err != nil {
		return err
	}
	if sum != r.SHA256 {
		return fmt.Errorf("%w: %s", ErrChecksum, r.Version)
	}
	return nil
}

func prune(dir, name, keep string) error {
	list, err := List(dir, name)
	if err != nil {
		return err
	}
	kept := 0
	for _, r := range list {
		if r.Version == keep || kept < KeepCount {
			kept++
			continue
		}
		for _, p := range []string{r.Path, r.Path + sumSuffix, r.Path + metaSuffix} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("prune %s: %w", p, err)
			}
		}
	}
	return nil
}

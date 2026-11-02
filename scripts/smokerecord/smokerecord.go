// Package smokerecord folds one passed smoke run into verified.json and
// images.lock.json. Dev and CI tooling only; output is deterministic.
package smokerecord

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// Run is one passed smoke run for a template.
type Run struct {
	Arch       string
	HTTPStatus int
	RunURL     string
	// Digests maps each pinned image ref to its registry digest.
	Digests map[string]string
}

// Apply updates the verified file and the lock file for t. The primary
// image digest goes on the record; every digest goes on the lock.
func Apply(verifiedPath, lockPath string, t catalog.Template, run Run) error {
	refs, ok := catalog.PinnedImages(t)
	if !ok {
		return fmt.Errorf("template %s has unpinned images and cannot be recorded", t.ID)
	}
	if run.Arch == "" {
		return fmt.Errorf("template %s: arch is required", t.ID)
	}
	if err := mergeLock(lockPath, run.Digests); err != nil {
		return err
	}
	return catalog.RecordVerified(verifiedPath, t, catalog.VerifiedRun{
		Arch: run.Arch, Digest: run.Digests[refs[0]], HTTPStatus: run.HTTPStatus, RunURL: run.RunURL,
	})
}

func mergeLock(path string, digests map[string]string) error {
	lock := catalog.LockFile{Images: map[string]catalog.LockedImage{}, Admission: map[string]catalog.Admission{}}
	if raw, err := os.ReadFile(path); err == nil { //nolint:gosec // path is a dev-tool flag
		if err := json.Unmarshal(raw, &lock); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
	}
	if lock.Images == nil {
		lock.Images = map[string]catalog.LockedImage{}
	}
	for ref, d := range digests {
		if d == "" {
			continue
		}
		li := lock.Images[ref]
		li.Digest = d
		lock.Images[ref] = li
	}
	out, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644) //nolint:gosec // committed data, world-readable by design
}

// ParseRefs reads lines of "ref digest", the format the admission job writes.
func ParseRefs(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.HasPrefix(f[1], "sha256:") {
			out[f[0]] = f[1]
		}
	}
	return out
}

// CraneDigests resolves every ref with crane, which must be on PATH.
func CraneDigests(ctx context.Context, refs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, ref := range refs {
		b, err := exec.CommandContext(ctx, "crane", "digest", ref).Output() //nolint:gosec // dev tool: fixed binary, refs come from the catalog
		if err != nil {
			return nil, fmt.Errorf("crane digest %s: %w", ref, err)
		}
		out[ref] = strings.TrimSpace(string(b))
	}
	return out, nil
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IdentityFile persists an agent Identity as JSON (0600). Writes go to a
// temp file that is fsynced and renamed over the target, so a crash leaves
// either the old or the new identity on disk, never a torn one. During a
// renewal the previous identity is kept beside it as <path>.prev until the
// new certificate has been proven to work.
type IdentityFile struct {
	Path string

	// rename is os.Rename; tests replace it to simulate a crash.
	rename func(oldpath, newpath string) error
}

// NewIdentityFile returns an IdentityFile for path.
func NewIdentityFile(path string) *IdentityFile {
	return &IdentityFile{Path: path, rename: os.Rename}
}

// identityFileFormat is the on-disk shape, unchanged since the first agent
// release so existing identity files keep loading.
type identityFileFormat struct {
	NodeID        string `json:"node_id"`
	ClientCertPEM []byte `json:"client_cert_pem"`
	ClientKeyPEM  []byte `json:"client_key_pem"`
	CACertPEM     []byte `json:"ca_cert_pem"`
}

const (
	identityPrevSuffix = ".prev"
	identityTempInfix  = ".tmp-"
)

// PrevPath is where the pre-renewal identity is kept.
func (f *IdentityFile) PrevPath() string { return f.Path + identityPrevSuffix }

// Load reads the identity and removes temp files a crashed write left
// behind. It returns an error wrapping os.ErrNotExist when none exists.
func (f *IdentityFile) Load() (*Identity, error) {
	f.removeStaleTemps()
	return readIdentity(f.Path)
}

// LoadPrev reads the pre-renewal identity kept at PrevPath.
func (f *IdentityFile) LoadPrev() (*Identity, error) {
	return readIdentity(f.PrevPath())
}

// Save atomically replaces the identity on disk with id.
func (f *IdentityFile) Save(id *Identity) error {
	data, err := json.MarshalIndent(identityFileFormat{
		NodeID: id.NodeID, ClientCertPEM: id.ClientCertPEM, ClientKeyPEM: id.ClientKeyPEM, CACertPEM: id.CACertPEM,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: encode identity: %w", err)
	}
	return f.writeAtomic(f.Path, data)
}

// Stage keeps the current identity file as PrevPath, then saves next.
// Confirm or Rollback completes it.
func (f *IdentityFile) Stage(next *Identity) error {
	cur, err := os.ReadFile(f.Path)
	if err != nil {
		return fmt.Errorf("agent: read current identity for backup: %w", err)
	}
	if err := f.writeAtomic(f.PrevPath(), cur); err != nil {
		return fmt.Errorf("agent: back up current identity: %w", err)
	}
	return f.Save(next)
}

// Confirm drops the pre-renewal backup once the new identity works.
func (f *IdentityFile) Confirm() error {
	if err := os.Remove(f.PrevPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("agent: remove identity backup: %w", err)
	}
	return nil
}

// Rollback restores the pre-renewal identity over the current one.
func (f *IdentityFile) Rollback() error {
	if err := f.rename(f.PrevPath(), f.Path); err != nil {
		return fmt.Errorf("agent: restore identity backup: %w", err)
	}
	return syncDir(filepath.Dir(f.Path))
}

// HasStaged reports whether a renewal was staged but never confirmed or
// rolled back, meaning the agent stopped in between.
func (f *IdentityFile) HasStaged() bool {
	_, err := os.Stat(f.PrevPath())
	return err == nil
}

// ResolveStaged settles a renewal the agent stopped in the middle of: the
// staged identity is kept if check accepts it, otherwise the backup is
// restored. It returns the identity to use.
func (f *IdentityFile) ResolveStaged(ctx context.Context, check func(context.Context, *Identity) error) (*Identity, error) {
	cur, err := f.Load()
	if err != nil {
		return nil, err
	}
	if !f.HasStaged() {
		return cur, nil
	}
	if checkErr := check(ctx, cur); checkErr == nil {
		return cur, f.Confirm()
	} else if !isAuthRejection(checkErr) {
		// Control plane unreachable: keep both and decide next start.
		return cur, nil
	}
	if err := f.Rollback(); err != nil {
		return nil, err
	}
	return f.Load()
}

func (f *IdentityFile) writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+identityTempInfix+"*")
	if err != nil {
		return fmt.Errorf("agent: create temp identity file: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: chmod temp identity file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: write temp identity file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: fsync temp identity file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("agent: close temp identity file: %w", err)
	}
	if err := f.rename(tmpName, path); err != nil {
		return fmt.Errorf("agent: rename identity file into place: %w", err)
	}
	committed = true
	return syncDir(dir)
}

func (f *IdentityFile) removeStaleTemps() {
	matches, err := filepath.Glob(f.Path + identityTempInfix + "*")
	if err != nil {
		return
	}
	prevMatches, _ := filepath.Glob(f.PrevPath() + identityTempInfix + "*")
	for _, m := range append(matches, prevMatches...) {
		if strings.Contains(filepath.Base(m), identityTempInfix) {
			_ = os.Remove(m)
		}
	}
}

func readIdentity(path string) (*Identity, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-controlled config path, not user input
	if err != nil {
		return nil, err
	}
	var f identityFileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("agent: parse identity file %s: %w", path, err)
	}
	return &Identity{NodeID: f.NodeID, ClientCertPEM: f.ClientCertPEM, ClientKeyPEM: f.ClientKeyPEM, CACertPEM: f.CACertPEM}, nil
}

// syncDir fsyncs dir so a completed rename survives power loss.
func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // the identity file's own directory
	if err != nil {
		return fmt.Errorf("agent: open identity dir for fsync: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return fmt.Errorf("agent: fsync identity dir: %w", err)
	}
	return nil
}

// Package desktopmode lets the desktop app trade a per-install token for a
// dashboard session over authenticated loopback, and pins Host and Origin.
package desktopmode

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

const (
	tokenFileName = "desktop-token"
	hashFileName  = "desktop-token.sha256"
	tokenPrefix   = "dsk_"
	tokenBytes    = 32
)

// TokenPath is the file the host side reads the token from, mode 0600.
func TokenPath(dataDir string) string { return filepath.Join(dataDir, tokenFileName) }

func hashPath(dataDir string) string { return filepath.Join(dataDir, hashFileName) }

// Tokens owns the per-install token. Only its hash stays in memory and in
// the hash file; the plaintext lives in the handoff file for the host app.
type Tokens struct {
	dir string
	mu  sync.RWMutex
	sum string
}

// NewTokens loads the stored hash, or creates a token on first use. A handoff file that is
// missing, not a plain 0600 file, or not matching the hash (a crash between the two writes)
// mints a fresh token, since the host could not use the old one.
func NewTokens(dataDir string) (*Tokens, error) {
	t := &Tokens{dir: dataDir}
	raw, err := os.ReadFile(hashPath(dataDir))
	switch {
	case err == nil:
		t.sum = string(raw)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("desktopmode: read token hash: %w", err)
	}
	if t.sum == "" || !handoffIntact(dataDir, t.sum) {
		if err := t.Rotate(); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// Verify reports whether presented is the current token, in constant time.
func (t *Tokens) Verify(presented string) bool {
	t.mu.RLock()
	sum := t.sum
	t.mu.RUnlock()
	if sum == "" || presented == "" {
		return false
	}
	return tokenhash.Matches(presented, sum)
}

// Rotate replaces the token and rewrites both files atomically; the old
// token stops working at once.
func (t *Tokens) Rotate() error {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("desktopmode: generate token: %w", err)
	}
	plain := tokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	sum := tokenhash.Sum(plain)
	t.mu.Lock()
	defer t.mu.Unlock()
	// Hash first: a crash between the writes leaves a handoff file that fails closed.
	if err := writeAtomic(hashPath(t.dir), []byte(sum)); err != nil {
		return err
	}
	if err := writeAtomic(TokenPath(t.dir), []byte(plain+"\n")); err != nil {
		return err
	}
	t.sum = sum
	return nil
}

// Revoke deletes the token and its hash. It lasts until the next boot, which creates a
// fresh token like a first run; to keep desktop mode closed, turn APP_DESKTOP_MODE off.
func (t *Tokens) Revoke() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sum = ""
	for _, p := range []string{hashPath(t.dir), TokenPath(t.dir)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("desktopmode: remove %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

// writeAtomic writes via a same-directory temp file so a reader never sees a partial token.
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".desktop-*")
	if err != nil {
		return fmt.Errorf("desktopmode: create temp: %w", err)
	}
	tmp := f.Name()
	cleanup := func() { _ = os.Remove(tmp) }
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("desktopmode: chmod temp: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("desktopmode: write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("desktopmode: sync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("desktopmode: close temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return fmt.Errorf("desktopmode: rename token file: %w", err)
	}
	return nil
}

// handoffIntact reports whether the token file is a regular, unshared 0600 file whose hash is sum.
func handoffIntact(dataDir, sum string) bool {
	p := TokenPath(dataDir)
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		return false
	}
	raw, err := os.ReadFile(p) //nolint:gosec // fixed name inside the data dir
	if err != nil {
		return false
	}
	return tokenhash.Matches(strings.TrimSpace(string(raw)), sum)
}

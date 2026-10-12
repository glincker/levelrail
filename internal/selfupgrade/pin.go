package selfupgrade

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PinFileName is the marker, under the data dir, that pins this host to
// signed releases. install.sh writes the same key=value format.
const PinFileName = "release-signature.pin"

// Pin records the first release whose signature this host verified.
type Pin struct {
	Version    string
	VerifiedAt time.Time
}

// PinPath is the pin file's location under dataDir.
func PinPath(dataDir string) string { return filepath.Join(dataDir, PinFileName) }

// ReadPin reports whether dataDir holds a pin. A present but unreadable or
// empty file still pins: failing open would let a truncated file unpin.
func ReadPin(dataDir string) (Pin, bool, error) {
	raw, err := os.ReadFile(PinPath(dataDir)) //nolint:gosec // fixed name under the operator's data dir
	if errors.Is(err, fs.ErrNotExist) {
		return Pin{}, false, nil
	}
	if err != nil {
		return Pin{}, true, fmt.Errorf("read signature pin: %w", err)
	}
	var p Pin
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch key {
		case "version":
			p.Version = val
		case "verified_at":
			if at, perr := time.Parse(time.RFC3339, val); perr == nil {
				p.VerifiedAt = at
			}
		}
	}
	return p, true, nil
}

// WritePin atomically records p. World-readable so a non-root control plane
// can report it; only root can write the data dir.
func WritePin(dataDir string, p Pin) error {
	if p.VerifiedAt.IsZero() {
		p.VerifiedAt = time.Now()
	}
	body := fmt.Sprintf("version=%s\nverified_at=%s\n", p.Version, p.VerifiedAt.UTC().Format(time.RFC3339))
	tmp, err := os.CreateTemp(dataDir, PinFileName+".tmp-")
	if err != nil {
		return fmt.Errorf("write signature pin: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write signature pin: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write signature pin: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // a marker, not a secret
		return fmt.Errorf("write signature pin: %w", err)
	}
	if err := os.Rename(tmp.Name(), PinPath(dataDir)); err != nil {
		return fmt.Errorf("write signature pin: %w", err)
	}
	return nil
}

// RemovePin clears the pin and reports whether one existed.
func RemovePin(dataDir string) (bool, error) {
	err := os.Remove(PinPath(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("remove signature pin: %w", err)
	}
	return true, nil
}

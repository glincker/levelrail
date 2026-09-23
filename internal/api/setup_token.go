package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SetupTokenFilename is the file under the data dir holding the one-time first-admin token.
const SetupTokenFilename = "setup-token"

// UserCounter is the store surface EnsureSetupToken needs.
type UserCounter interface {
	CountUsers(ctx context.Context) (int, error)
}

// SetupTokenPath returns the setup token file path inside dataDir.
func SetupTokenPath(dataDir string) string {
	return filepath.Join(dataDir, SetupTokenFilename)
}

// ReadSetupToken returns the current setup token, or "" if none exists.
func ReadSetupToken(dataDir string) (string, error) {
	b, err := os.ReadFile(SetupTokenPath(dataDir)) //nolint:gosec // operator-controlled data dir
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("api: read setup token: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// EnsureSetupToken returns the setup token for an instance with no users,
// creating it (mode 0600) if missing; created reports whether it is new.
// Once any user exists the file is removed and "" is returned.
func EnsureSetupToken(ctx context.Context, users UserCounter, dataDir string) (token string, created bool, err error) {
	n, err := users.CountUsers(ctx)
	if err != nil {
		return "", false, fmt.Errorf("api: setup token: count users: %w", err)
	}
	if n > 0 {
		return "", false, removeSetupToken(dataDir)
	}
	existing, err := ReadSetupToken(dataDir)
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		if err := os.Chmod(SetupTokenPath(dataDir), 0o600); err != nil {
			return "", false, fmt.Errorf("api: restrict setup token permissions: %w", err)
		}
		return existing, false, nil
	}
	token, err = randomToken()
	if err != nil {
		return "", false, fmt.Errorf("api: generate setup token: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", false, fmt.Errorf("api: setup token: create data dir: %w", err)
	}
	path := SetupTokenPath(dataDir)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // operator-controlled data dir
	if err != nil {
		return "", false, fmt.Errorf("api: create setup token file: %w", err)
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		_ = f.Close()
		return "", false, fmt.Errorf("api: write setup token: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", false, fmt.Errorf("api: close setup token file: %w", err)
	}
	return token, true, nil
}

func removeSetupToken(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	if err := os.Remove(SetupTokenPath(dataDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("api: remove setup token: %w", err)
	}
	return nil
}

// setupTokenValid reports whether presented matches the on-disk token.
// No data dir or no token file means registration is closed.
func (rt *Router) setupTokenValid(presented string) (bool, error) {
	if rt.dataDir == "" || presented == "" {
		return false, nil
	}
	want, err := ReadSetupToken(rt.dataDir)
	if err != nil || want == "" {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(want)) == 1, nil
}

type setupStatusResponse struct {
	NeedsSetup bool `json:"needs_setup"`
}

func (rt *Router) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := rt.auth.CountUsers(r.Context())
	if err != nil {
		rt.internalError(w, "api: setup status: count users failed", err)
		return
	}
	writeJSON(w, http.StatusOK, setupStatusResponse{NeedsSetup: n == 0})
}

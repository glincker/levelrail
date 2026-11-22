//go:build windows

package agentupdate

import (
	"errors"
	"os"
	"time"
)

// ErrBusy means another install or rollback holds the lock.
var ErrBusy = errors.New("another agent update or rollback is already running")

const staleLock = 15 * time.Minute

// lock uses an exclusive-create file, dropping one left behind by a crash.
func lock(path string) (func(), error) {
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) > staleLock {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // lock file next to the agent executable
	if err != nil {
		return nil, ErrBusy
	}
	_ = f.Close()
	return func() { _ = os.Remove(path) }, nil
}

func syncDir(string) {}

//go:build !windows

package agentupdate

import (
	"errors"
	"os"
	"syscall"
)

// ErrBusy means another install or rollback holds the lock.
var ErrBusy = errors.New("another agent update or rollback is already running")

// lock takes an exclusive advisory lock beside the executable.
func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lock file next to the agent executable
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { //nolint:gosec // fd fits int
		_ = f.Close()
		return nil, ErrBusy
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:gosec // fd fits int
		_ = f.Close()
	}, nil
}

// syncDir makes the renames durable.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil { //nolint:gosec // directory of the agent executable
		_ = d.Sync()
		_ = d.Close()
	}
}

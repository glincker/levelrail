package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// ssh appends a dot and 16 random characters to its control socket path.
	sshSocketSuffix = 17
	sshSocketName   = "ssh.sock"
	shortLimaDir    = ".lr-lima"
	limaHomeMode    = 0o700
)

// socketPathLen is the longest socket path Lima will bind for an instance in home.
func socketPathLen(home, instance string) int {
	return len(filepath.Join(home, instance, sshSocketName)) + sshSocketSuffix
}

// chooseLimaHome keeps the preferred Lima home when its sockets fit under maxPath, else falls
// back to a short directory in the user's home. short reports the fallback.
func chooseLimaHome(preferred, userHome, instance string, maxPath int) (home string, short bool, err error) {
	if socketPathLen(preferred, instance) < maxPath {
		return preferred, false, nil
	}
	alt := filepath.Join(userHome, shortLimaDir)
	if socketPathLen(alt, instance) >= maxPath {
		return "", false, fmt.Errorf("no Lima home fits the %d character socket path limit (home %q, instance %q): use a shorter user name or VM name", maxPath, userHome, instance)
	}
	return alt, true, nil
}

// ensureLimaHome creates home as a private real directory. A symlink there is refused so
// another process cannot redirect the VM state.
func ensureLimaHome(home string) error {
	if fi, err := os.Lstat(home); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return fmt.Errorf("%s must be a real directory, not a symlink or file", home)
		}
		return os.Chmod(home, limaHomeMode)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(home), limaHomeMode); err != nil {
		return err
	}
	if err := os.Mkdir(home, limaHomeMode); err != nil {
		return err
	}
	return os.Chmod(home, limaHomeMode)
}

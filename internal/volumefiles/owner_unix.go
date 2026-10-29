//go:build !windows

package volumefiles

import (
	"errors"
	"io/fs"
	"syscall"
)

// matchOwner gives the temp file the uid and gid of ref (the file it
// replaces, or its directory) so a non-root app can still use it.
func (s *Store) matchOwner(ref, tmp string) error {
	st, err := s.root.Lstat(ref)
	if err != nil {
		return mapErr(err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	err = s.root.Lchown(tmp, int(sys.Uid), int(sys.Gid))
	if err != nil && !errors.Is(err, fs.ErrPermission) {
		return mapErr(err)
	}
	return nil
}

//go:build !windows

package proxyroutes

import (
	"fmt"
	"os"
	"syscall"
)

// openNoFollow refuses to open a symlink as the final path element.
const openNoFollow = syscall.O_NOFOLLOW

// ownerNote describes the directory owner for the not-writable message.
func ownerNote(fi os.FileInfo) string {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf(", the directory is owned by uid %d gid %d with mode %s", st.Uid, st.Gid, fi.Mode().Perm())
	}
	return ""
}

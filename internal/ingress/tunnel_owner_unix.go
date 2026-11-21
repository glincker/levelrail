//go:build !windows

package ingress

import (
	"os"
	"syscall"
)

// ownedByCurrentUser keeps a directory another user created from holding our sockets.
func ownedByCurrentUser(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

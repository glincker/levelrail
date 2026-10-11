//go:build !windows

package dockerguard

import (
	"os"
	"syscall"
)

// ownedByOtherUser reports the owning uid when info is owned by someone
// other than this process.
func ownedByOtherUser(info os.FileInfo) (uid int, other bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	uid = int(st.Uid)
	return uid, uid != os.Geteuid()
}

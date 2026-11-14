//go:build linux

package cloudiac

import "syscall"

const tmpfsMagic = 0x01021994

// isTmpfs reports whether dir sits on tmpfs, so the plaintext state never reaches a disk.
func isTmpfs(dir string) bool {
	var st syscall.Statfs_t
	return syscall.Statfs(dir, &st) == nil && int64(st.Type) == tmpfsMagic
}

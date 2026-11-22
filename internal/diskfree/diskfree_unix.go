//go:build !windows

package diskfree

import "syscall"

// Stat returns Bavail based free space, not Bfree, so root-reserved blocks are
// not counted as usable.
func Stat(path string) (Info, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Info{}, err
	}
	bs := int64(st.Bsize) //nolint:gosec // statfs fields are non-negative
	return Info{
		Free:        int64(st.Bavail) * bs, //nolint:gosec // statfs fields are non-negative
		Total:       int64(st.Blocks) * bs, //nolint:gosec // statfs fields are non-negative
		InodesFree:  uint64(st.Ffree),      //nolint:gosec // statfs fields are non-negative
		InodesTotal: uint64(st.Files),      //nolint:gosec // statfs fields are non-negative
	}, nil
}

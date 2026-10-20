package api

import (
	"fmt"
	"syscall"
)

type syscallStatfs = syscall.Statfs_t

func statfsBytes(dir string, st *syscallStatfs) (free, total int64, err error) {
	if dir == "" {
		return 0, 0, fmt.Errorf("data dir is not configured")
	}
	if err := syscall.Statfs(dir, st); err != nil {
		return 0, 0, err
	}
	bs := int64(st.Bsize)                                    //nolint:gosec // statfs fields are non-negative
	return int64(st.Bavail) * bs, int64(st.Blocks) * bs, nil //nolint:gosec // statfs fields are non-negative
}

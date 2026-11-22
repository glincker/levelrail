//go:build !windows

package serverready

import "syscall"

func diskFreeGB(path string) (float64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return float64(st.Bavail) * float64(st.Bsize) / (1 << 30), nil //nolint:gosec // statfs fields are non-negative
}

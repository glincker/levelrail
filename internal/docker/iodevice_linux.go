//go:build linux

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// blockDeviceFor returns the whole-disk device path backing dir, or "" when it
// cannot be resolved (overlay or network storage). Throttling needs the disk, not a partition.
func blockDeviceFor(dir string) string {
	var st syscall.Stat_t
	if err := syscall.Stat(dir, &st); err != nil {
		return ""
	}
	dev := uint64(st.Dev) //nolint:unconvert // Dev is uint32 on some arches
	major := uint32(((dev >> 8) & 0xfff) | ((dev >> 32) &^ 0xfff))
	minor := uint32((dev & 0xff) | ((dev >> 12) &^ 0xff))
	link, err := filepath.EvalSymlinks(fmt.Sprintf("/sys/dev/block/%d:%d", major, minor))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(link, "partition")); err == nil {
		link = filepath.Dir(link)
	}
	path := "/dev/" + filepath.Base(link)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

//go:build windows

package diskfree

import "golang.org/x/sys/windows"

// Stat uses the free bytes available to the calling user. Windows has no inode
// counts, so those stay zero.
func Stat(path string) (Info, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Info{}, err
	}
	var avail, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &totalFree); err != nil {
		return Info{}, err
	}
	return Info{Free: int64(avail), Total: int64(total)}, nil //nolint:gosec // disk sizes fit int64
}

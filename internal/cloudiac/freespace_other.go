//go:build !linux && !darwin

package cloudiac

// FreeBytes always refuses here, so the fetch never runs without a disk floor check.
func FreeBytes(string) (int64, error) { return 0, ErrFreeSpaceUnsupported }

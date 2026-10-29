//go:build windows

package backup

// freeBytes is not reported on Windows control planes.
func freeBytes(string) (uint64, bool) { return 0, false }

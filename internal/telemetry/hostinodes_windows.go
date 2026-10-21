//go:build windows

package telemetry

// HostInodes has no Windows equivalent, so it reports zero totals like a
// filesystem without inode accounting.
func HostInodes(string) (total, free uint64, err error) {
	return 0, 0, nil
}

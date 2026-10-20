//go:build !darwin

package telemetry

// platformHostMemory is only reached when /proc/meminfo is missing, so
// there is no other source on these platforms.
func platformHostMemory() (totalBytes, availableBytes int64, err error) {
	return 0, 0, ErrHostMemoryUnsupported
}

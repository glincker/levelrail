//go:build !linux

package docker

// blockDeviceFor is Linux only; elsewhere IO caps are not applied.
func blockDeviceFor(string) string { return "" }

//go:build windows

package main

// Capacity is unknown on Windows, so no volume samples are written there.
func filesystemTotalBytes(string) int64 { return 0 }

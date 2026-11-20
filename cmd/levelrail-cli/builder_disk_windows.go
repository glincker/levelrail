//go:build windows

package main

// freeDiskGiB is unknown on Windows, where the builder is not supported yet.
func freeDiskGiB(string) int { return 0 }

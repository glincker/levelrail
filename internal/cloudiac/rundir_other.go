//go:build !linux

package cloudiac

// isTmpfs is unknown here, so the run reports that its dir may be disk backed.
func isTmpfs(string) bool { return false }

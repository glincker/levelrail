//go:build windows

package footprint

// cpuSeconds is unavailable on Windows, where the CLI never reports a footprint.
func cpuSeconds() float64 { return 0 }

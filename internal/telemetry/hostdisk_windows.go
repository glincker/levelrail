//go:build windows

package telemetry

import "errors"

// diskSpaceBytes is unavailable on Windows, where the CLI never samples host disk.
func diskSpaceBytes(string) (total, used int64, err error) {
	return 0, 0, errors.New("telemetry: host disk usage is not supported on windows")
}

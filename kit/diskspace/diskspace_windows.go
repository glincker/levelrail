//go:build windows

package diskspace

import "errors"

var errUnsupported = errors.New("diskspace: not supported on windows")

// Free is unsupported on Windows, where only the CLI is built.
func Free(string) (int64, error) { return 0, errUnsupported }

// Usage is unsupported on Windows, where only the CLI is built.
func Usage(string) (free, total int64, err error) { return 0, 0, errUnsupported }

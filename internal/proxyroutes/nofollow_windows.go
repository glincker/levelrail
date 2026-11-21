//go:build windows

package proxyroutes

import "os"

// openNoFollow is zero on Windows: read() already rejects non-regular files
// with Lstat, and the proxy directory feature targets unix hosts.
const openNoFollow = 0

// ownerNote has no uid or gid to report on Windows.
func ownerNote(_ os.FileInfo) string { return "" }

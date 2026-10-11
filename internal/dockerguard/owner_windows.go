//go:build windows

package dockerguard

import "os"

// ownedByOtherUser has no uid concept on Windows; the directory ACL is the
// boundary there.
func ownedByOtherUser(os.FileInfo) (uid int, other bool) { return 0, false }

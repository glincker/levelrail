//go:build windows

package ingress

import "os"

// ownedByCurrentUser: Windows has no unix owner bits, the directory ACL decides.
func ownedByCurrentUser(os.FileInfo) bool { return true }

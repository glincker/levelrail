package spec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// forbiddenBindMountPaths mirrors internal/compose's own list of the same
// name (compose.go): kept as a separate copy, not imported, because
// internal/compose already imports internal/spec (a compose service
// expands into a spec.Service), so this package importing back would
// cycle. Keep the two lists in sync by hand if either ever changes.
var forbiddenBindMountPaths = []string{
	"/",
	"/etc",
	"/root",
	"/boot",
	"/sys",
	"/proc",
	"/var/lib/docker",
	"/var/run/docker.sock",
	"/var/run",
}

// validateBindMountHostPath mirrors internal/compose's own function of
// the same name: rejects a relative path and every path
// forbiddenBindMountPaths covers, an exact match or a match of clean+"/"
// as a prefix.
func validateBindMountHostPath(hostPath string) error {
	if !strings.HasPrefix(hostPath, "/") {
		return fmt.Errorf("bind-mount host path %q must be an absolute path", hostPath)
	}
	clean := filepath.Clean(hostPath)
	for _, forbidden := range forbiddenBindMountPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("bind-mount host path %q is not allowed: %q is a protected system path", hostPath, forbidden)
		}
	}
	return nil
}

// Package bindmount validates a service's bind-mount host path, shared
// by internal/compose and internal/spec: internal/compose imports
// internal/spec (a compose service expands into a spec.Service), so
// neither of those two packages can hold this logic without the other
// importing it back and cycling. This package depends on neither.
package bindmount

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ForbiddenPaths are host paths a bind mount may never target, enforced
// even for an AbilityRoot caller (internal/api's ability gate, not this
// list, is the primary boundary; this is defense in depth): an exact
// match or a match of clean+"/" as a prefix. Each one grants something
// categorically worse than ordinary bind-mount access, host root
// compromise for most of these. /var/run/docker.sock (and /var/run
// generally, since a socket can be bind-mounted from anywhere under it)
// is deliberately excluded from this feature by design, not an
// oversight: Docker-socket access is a full container-escape-to-host-
// root vector via the Docker API, a categorically different and
// unreviewed capability that needs its own explicit design decision
// later, not bundled into general bind-mount support here.
var ForbiddenPaths = []string{
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

// ValidateHostPath rejects a relative path and every path ForbiddenPaths
// covers; anything else is a real, operator-owned host directory this
// feature exists to allow.
func ValidateHostPath(hostPath string) error {
	if !strings.HasPrefix(hostPath, "/") {
		return fmt.Errorf("bind-mount host path %q must be an absolute path", hostPath)
	}
	clean := filepath.Clean(hostPath)
	for _, forbidden := range ForbiddenPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("bind-mount host path %q is not allowed: %q is a protected system path", hostPath, forbidden)
		}
	}
	return nil
}

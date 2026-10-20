// Package bindmount validates a service's bind-mount host path, shared
// by internal/compose and internal/spec: internal/compose imports
// internal/spec (a compose service expands into a spec.Service), so
// neither of those two packages can hold this logic without the other
// importing it back and cycling. This package depends on neither.
package bindmount

import (
	"fmt"
	"os"
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
	"/run",
	"/dev",
	"/lib/modules",
	"/var/lib/containerd",
	"/var/lib/kubelet",
	"/var/lib/levelrail-data",
	"/var/lib/levelrail-agent-data",
	"/bin",
	"/sbin",
	"/lib",
	"/lib64",
	"/usr/bin",
	"/usr/sbin",
	"/usr/lib",
	"/usr/local/bin",
	"/usr/local/sbin",
	"/var/spool/cron",
}

// forbiddenPaths is ForbiddenPaths plus the data directory this process was
// actually configured with, since the default path is only one possibility.
func forbiddenPaths() []string {
	out := ForbiddenPaths
	if d := strings.TrimSpace(os.Getenv("APP_DATA_DIR")); d != "" && filepath.IsAbs(d) {
		out = append(append([]string(nil), out...), filepath.Clean(d))
	}
	return out
}

// ValidateHostPath rejects a relative path and every path ForbiddenPaths
// covers; anything else is a real, operator-owned host directory this
// feature exists to allow.
func ValidateHostPath(hostPath string) error {
	if !strings.HasPrefix(hostPath, "/") {
		return fmt.Errorf("bind-mount host path %q must be an absolute path", hostPath)
	}
	clean := filepath.Clean(hostPath)
	for _, forbidden := range forbiddenPaths() {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("bind-mount host path %q is not allowed: %q is a protected system path", hostPath, forbidden)
		}
		// An ancestor mount (/var, /var/lib) exposes the protected path beneath it.
		if forbidden != "/" && strings.HasPrefix(forbidden, strings.TrimSuffix(clean, "/")+"/") {
			return fmt.Errorf("bind-mount host path %q is not allowed: it contains the protected system path %q", hostPath, forbidden)
		}
	}
	return nil
}

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
	"/var/log",
	"/usr/local/etc",
}

// credentialDirs are per-user secret stores; a mount of or under one hands
// the container that user's keys wherever the home directory lives.
var credentialDirs = map[string]bool{
	".ssh": true, ".gnupg": true, ".aws": true, ".kube": true, ".docker": true,
}

// EnvAllowedRoots lists colon-separated absolute directories; when set,
// bind mounts must sit inside one of them.
const EnvAllowedRoots = "APP_BIND_ALLOWED_ROOTS"

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
	if err := rejectCredentialPaths(hostPath, clean); err != nil {
		return err
	}
	return checkAllowedRoots(hostPath, clean)
}

// rejectCredentialPaths blocks credential directories and bare home directories,
// whose children include them.
func rejectCredentialPaths(hostPath, clean string) error {
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	for _, part := range parts {
		if credentialDirs[part] {
			return fmt.Errorf("bind-mount host path %q is not allowed: it is or sits inside a credential directory (%s)", hostPath, part)
		}
	}
	if parts[0] == "home" && len(parts) <= 2 {
		return fmt.Errorf("bind-mount host path %q is not allowed: mount a subdirectory of a home directory, not the home directory itself", hostPath)
	}
	return nil
}

// checkAllowedRoots enforces the operator allowlist when one is configured.
func checkAllowedRoots(hostPath, clean string) error {
	raw := strings.TrimSpace(os.Getenv(EnvAllowedRoots))
	if raw == "" {
		return nil
	}
	for _, root := range strings.Split(raw, ":") {
		root = filepath.Clean(strings.TrimSpace(root))
		if filepath.IsAbs(root) && root != "/" && (clean == root || strings.HasPrefix(clean, root+"/")) {
			return nil
		}
	}
	return fmt.Errorf("bind-mount host path %q is outside the directories allowed by %s", hostPath, EnvAllowedRoots)
}

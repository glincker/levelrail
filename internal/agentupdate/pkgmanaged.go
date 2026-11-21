package agentupdate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrPackageManaged means a package manager owns the agent binary, so an
// in-place replace would be overwritten or fight the package database.
var ErrPackageManaged = errors.New("the agent binary is managed by a package manager; update it with that package manager instead")

// PackageManagedHelp is the next step printed when a package manager owns the agent.
const PackageManagedHelp = "update it with your package manager (for example apt-get install --only-upgrade levelrail-agent, dnf upgrade levelrail-agent or brew upgrade levelrail-agent), or run: levelrail-agent self-update --apply --allow-package-managed"

const (
	sourceFile   = "install-source"
	ownerTimeout = 3 * time.Second
)

var packageDirs = []string{"/usr/bin/", "/usr/sbin/", "/usr/local/Cellar/", "/opt/homebrew/Cellar/"}

// PackageManaged reports whether exe is installed by a package manager: by
// location, by an install-source marker reading "package", or by dpkg or rpm.
func PackageManaged(exe, dataDir string) bool {
	return packageManagedWith(exe, dataDir, runtime.GOOS, ownedByPackage)
}

func packageManagedWith(exe, dataDir, goos string, owned func(string) bool) bool {
	p := filepath.ToSlash(exe)
	for _, d := range packageDirs {
		if strings.HasPrefix(p, d) {
			return true
		}
	}
	if dataDir != "" {
		if b, err := os.ReadFile(filepath.Join(filepath.Clean(dataDir), sourceFile)); err == nil && strings.TrimSpace(string(b)) == "package" {
			return true
		}
	}
	return goos == "linux" && owned != nil && owned(exe)
}

// ownedByPackage asks dpkg then rpm with a fixed argv; a missing tool means no.
func ownedByPackage(exe string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), ownerTimeout)
	defer cancel()
	for _, argv := range [][]string{{"dpkg", "-S", "--", exe}, {"rpm", "-qf", "--", exe}} {
		if _, err := exec.LookPath(argv[0]); err != nil {
			continue
		}
		if exec.CommandContext(ctx, argv[0], argv[1:]...).Run() == nil { //nolint:gosec // fixed argv, exe is our own resolved path
			return true
		}
	}
	return false
}

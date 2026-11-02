package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// EnvPlatformMismatch picks what happens when an image has no manifest for the
// node architecture: refuse-when-no-emulation (default), warn or refuse.
const EnvPlatformMismatch = "APP_PLATFORM_MISMATCH"

const (
	platformRefuseNoEmu = "refuse-when-no-emulation"
	platformWarn        = "warn"
	platformRefuse      = "refuse"
)

// emulation is what is known about running another arch on this node.
type emulation int

const (
	emulationUnknown emulation = iota
	emulationAvailable
	emulationMissing
)

// binfmtDir is where the kernel lists registered binary handlers; a variable
// so tests can point it at a temp dir.
var binfmtDir = "/proc/sys/fs/binfmt_misc"

func platformMismatchMode() string {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvPlatformMismatch))); v {
	case platformWarn, platformRefuse:
		return v
	}
	return platformRefuseNoEmu
}

// binfmtNames maps an OCI arch to the handler names that run it.
var binfmtNames = map[string][]string{
	"amd64":   {"qemu-x86_64", "rosetta"},
	"arm64":   {"qemu-aarch64"},
	"arm":     {"qemu-arm"},
	"386":     {"qemu-i386"},
	"riscv64": {"qemu-riscv64"},
	"s390x":   {"qemu-s390x"},
	"ppc64le": {"qemu-ppc64le"},
}

// emulationFor reads binfmt_misc. A missing directory or unknown arch is
// undeterminable (the daemon may be on another host), not "missing".
func emulationFor(arch string) emulation {
	names, ok := binfmtNames[arch]
	if !ok {
		return emulationUnknown
	}
	if _, err := os.Stat(binfmtDir); err != nil {
		return emulationUnknown
	}
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(binfmtDir, n)) //nolint:gosec // fixed handler names under a fixed dir
		if err == nil && strings.HasPrefix(strings.TrimSpace(string(b)), "enabled") {
			return emulationAvailable
		}
	}
	return emulationMissing
}

// platformVerdict returns the user-facing text and whether to refuse the deploy.
func platformVerdict(image, imgArch string, node docker.NodePlatform, mode string, emu emulation) (string, bool) {
	head := fmt.Sprintf("image %s is %s but this node is %s/%s", image, imgArch, node.OS, node.Arch)
	fix := "; pick an image tag with a " + node.Arch + " manifest, or install qemu binfmt emulation on the node (for example the tonistiigi/binfmt image)"
	switch {
	case mode == platformRefuse:
		return head + " and " + EnvPlatformMismatch + "=refuse" + fix, true
	case mode == platformWarn:
		return head + "; it may fail with exec format error or run slowly under emulation", false
	case emu == emulationAvailable:
		return head + "; it runs under emulation and may be slow", false
	case emu == emulationMissing:
		return head + " and the node has no emulation for " + imgArch + fix, true
	}
	return head + "; emulation could not be checked, so it may fail with exec format error or run slowly", false
}

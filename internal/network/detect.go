package network

// This file: ADR 006's "kernel module detection used to take the faster
// in-kernel path when it's available."
//
// Detection is separated from use, and takes its evidence through an
// injectable Probe rather than reading the filesystem directly, for one
// concrete reason: the decision "which backend should this node use" has
// several branches that each matter operationally (an operator who
// expected the kernel path and silently got userspace has a performance
// mystery, not an error), and none of those branches are testable at all
// if the function reads /sys and /proc itself. With a Probe, every branch
// is a table row.

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Probe supplies the evidence Detect reasons over. The real
// implementation (SystemProbe) reads the local filesystem; tests supply
// values directly.
//
// An interface rather than three function fields because the three
// signals are only meaningful together, and a caller that supplied two
// of three would get a nonsense answer from a zero-valued third.
type Probe interface {
	// OS is the GOOS this node is running. WireGuard's kernel module is
	// a Linux thing; nothing else has one to detect.
	OS() string

	// KernelModuleLoaded reports whether the wireguard kernel module is
	// present and loaded. On Linux this is the existence of
	// /sys/module/wireguard.
	KernelModuleLoaded() bool

	// Privileged reports whether this process can create and configure a
	// network interface at all. A loaded kernel module is useless to an
	// unprivileged process, which is exactly the "restricted kernels,
	// some container-based VPS hosts" case ADR 006 cites as the reason
	// the userspace path exists.
	Privileged() bool
}

// DetectResult is Detect's full answer: which backend to use, and why.
// The reason is not decoration. "Userspace" alone is indistinguishable
// from a misconfiguration; "userspace because the kernel module is not
// loaded" is an operator's next action.
type DetectResult struct {
	Backend Backend
	Reason  string
}

// Detect chooses the WireGuard backend for this node.
//
// The order of the checks is the decision:
//
//  1. Not privileged: nothing works, not even userspace, because
//     wireguard-go still needs to create a TUN device. This is reported
//     as Disabled rather than as an error so a control plane that cannot
//     mesh still starts and still reconciles containers, degraded but
//     running. A platform that refuses to boot because it cannot bring up
//     a VPN it may not even need on a single-node install would be a
//     worse failure than the one it is reporting.
//  2. Kernel module loaded, on Linux: the fast path.
//  3. Everything else: wireguard-go. ADR 006's whole point is that this
//     is a supported outcome, not a fallback to apologize for.
func Detect(p Probe) DetectResult {
	if !p.Privileged() {
		return DetectResult{
			Backend: BackendDisabled,
			Reason:  "insufficient privileges to create a network interface: mesh networking is unavailable on this node",
		}
	}
	if p.OS() == "linux" && p.KernelModuleLoaded() {
		return DetectResult{
			Backend: BackendKernel,
			Reason:  "in-kernel wireguard module is loaded",
		}
	}
	if p.OS() != "linux" {
		return DetectResult{
			Backend: BackendUserspace,
			Reason:  fmt.Sprintf("no in-kernel wireguard module on %s: using the userspace implementation", p.OS()),
		}
	}
	return DetectResult{
		Backend: BackendUserspace,
		Reason:  "in-kernel wireguard module is not loaded: using the userspace implementation",
	}
}

// SystemProbe is the real Probe, reading this machine's own state.
type SystemProbe struct{}

// OS reports the compiled-in GOOS.
func (SystemProbe) OS() string { return runtime.GOOS }

// KernelModuleLoaded reports whether /sys/module/wireguard exists, which
// is how the kernel exposes a loaded module.
//
// Deliberately does not attempt to load the module (no modprobe, no
// shelling out, per CLAUDE.md 4.3's standing "never shell out" rule which
// applies here for the same reason it applies to Docker). Loading a
// kernel module is a host-configuration decision an operator makes, not
// something a deploy platform should do behind their back on a machine
// they may not fully control.
func (SystemProbe) KernelModuleLoaded() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := os.Stat("/sys/module/wireguard")
	return err == nil
}

// Privileged reports whether this process is likely able to create a
// network interface.
//
// Euid 0 is the check, with a deliberate caveat: on Linux, CAP_NET_ADMIN
// without full root is also sufficient, and this returns false for that
// case. That is a conservative wrong answer (a capable process is told it
// is not), chosen over the alternative because reading capabilities
// portably means parsing /proc/self/status and getting the bit position
// right, and being wrong in the other direction means the mesh reports
// itself up and then fails at interface creation. Detect's Reason string
// names privileges explicitly so an operator running with capabilities
// rather than root gets a message they can act on rather than silence.
func (SystemProbe) Privileged() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() == 0
}

// interfaceName is the network interface a node's mesh device is created
// as.
//
// Derived from the caller-supplied short name (CLAUDE.md section 3: no
// product name string in source) and truncated, because interface names
// are hard-limited to 15 characters on Linux (IFNAMSIZ-1) and a name that
// overflows fails at device creation with an error that does not mention
// length. The trailing digit leaves room for a second device later
// without changing the naming scheme.
func interfaceName(shortName string) string {
	const maxIfaceName = 15

	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return -1
		}
	}, shortName)

	if clean == "" {
		// Nothing usable in the brand short name (empty, or entirely
		// punctuation). "wg" is WireGuard's own universal convention and
		// is not a product name, so it is a safe last resort rather than
		// a hardcoded brand string.
		clean = "wg"
	}
	name := clean + "0"
	if len(name) > maxIfaceName {
		name = clean[:maxIfaceName-1] + "0"
	}
	return name
}

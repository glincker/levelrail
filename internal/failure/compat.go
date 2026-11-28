package failure

import "strings"

// Compat class ids: stable, shown by the API and CLI, never renamed.
const (
	CompatCapRefused          = "cap_refused"
	CompatSocketRefused       = "socket_refused"
	CompatArchMismatch        = "arch_mismatch"
	CompatPortInUse           = "port_in_use"
	CompatKernelModuleMissing = "kernel_module_missing"
	CompatDeviceMissing       = "device_missing"
)

// CompatClass names why a template cannot run on this host and what unlocks it.
type CompatClass struct {
	ID    string `json:"id"`
	Cause string `json:"cause"`
	Fix   string `json:"fix"`

	patterns []string
}

// compatClasses is checked in order, most specific first. Patterns are lowercase substrings.
var compatClasses = []CompatClass{
	{
		ID:       CompatSocketRefused,
		Cause:    "The template mounts the Docker socket and the docker_api grant is missing or off.",
		Fix:      "Turn on the Docker API capability in runtime settings, install again as an operator with the root ability and approve the grant.",
		patterns: []string{"mounting the docker socket", "disabled: docker_api", "docker_api grant", "bind_sensitive"},
	},
	{
		ID:       CompatDeviceMissing,
		Cause:    "The template needs a host device (or GPU) that is not granted or not present on this node.",
		Fix:      "Enable the devices capability in runtime settings and grant the device, or install on a node that has it (for a GPU, install the NVIDIA container toolkit).",
		patterns: []string{"disabled: devices", "devices_not_granted", "device_cgroup_rules", "devices allowlist", "needs an nvidia gpu", "could not select device driver", "error gathering device information", "no such device"},
	},
	{
		ID:       CompatArchMismatch,
		Cause:    "The template has no image built for this node's CPU architecture.",
		Fix:      "Install on a node with a matching architecture, pin an image tag that publishes this architecture, or repeat with allow_platform_mismatch to try emulation.",
		patterns: []string{"exec format error", "no matching manifest for linux", "does not match the detected host platform", " has no linux/", "wrong architecture"},
	},
	{
		ID:       CompatPortInUse,
		Cause:    "A host port the template publishes is already taken on this node.",
		Fix:      "Stop whatever holds the port, or change the published host port in the template settings, then install again.",
		patterns: []string{"port is already allocated", "address already in use", "bind: address"},
	},
	{
		ID:       CompatKernelModuleMissing,
		Cause:    "The host kernel lacks a module the template needs (for example WireGuard or iptables).",
		Fix:      "Load or install the kernel module on the node (modprobe, or a kernel package that ships it), or use a node image whose kernel includes it.",
		patterns: []string{"modprobe", "kernel module", "lib/modules", "no such module", "unknown filesystem type", "module not found", "operation not supported by the kernel"},
	},
	{
		ID:       CompatCapRefused,
		Cause:    "The template asks for a Linux capability or privilege that this instance does not allow.",
		Fix:      "Enable the matching capability class in runtime settings and install as an operator with the root ability, or pick a template that needs no extra capability.",
		patterns: []string{"cap_add_disallowed", "capability class is disabled", "is off in runtime settings", "review allowlist", "adds capability", "needs the root ability", "network_mode: host", "privileged"},
	},
}

// CompatClasses lists every compat class in match order.
func CompatClasses() []CompatClass {
	out := make([]CompatClass, len(compatClasses))
	copy(out, compatClasses)
	return out
}

// ClassifyCompat maps an install refusal or start error to a compat class.
func ClassifyCompat(text string) (CompatClass, bool) {
	low := strings.ToLower(text)
	for _, c := range compatClasses {
		for _, p := range c.patterns {
			if strings.Contains(low, p) {
				return c, true
			}
		}
	}
	return CompatClass{}, false
}

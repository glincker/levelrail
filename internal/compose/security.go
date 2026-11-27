package compose

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
)

// HostAccess holds the service keys that widen a container's reach into
// the host. None has a safe translation, so validate rejects every one
// that is set, citing the dockerguard rule the socket proxy would apply.
type HostAccess struct {
	Privileged  bool     `yaml:"privileged"`
	NetworkMode string   `yaml:"network_mode"`
	Pid         string   `yaml:"pid"`
	Ipc         string   `yaml:"ipc"`
	Uts         string   `yaml:"uts"`
	UsernsMode  string   `yaml:"userns_mode"`
	Cgroup      string   `yaml:"cgroup"`
	CapAdd      []string `yaml:"cap_add"`
	Devices     []any    `yaml:"devices"`
	SecurityOpt []string `yaml:"security_opt"`
	VolumesFrom []string `yaml:"volumes_from"`
}

// validateHostAccess reports one error per host-widening key a service sets.
func validateHostAccess(f *File) []error {
	var errs []error
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		for _, msg := range hostAccessViolations(svc, f.CapGrants[name], f.DropUngrantedCaps) {
			errs = append(errs, fmt.Errorf("service %q: %s", name, msg))
		}
		if err := hostNetworkLayout(f, name, svc); err != nil {
			errs = append(errs, err)
		}
		for _, v := range svc.Volumes {
			if v.HostPath == "" || !isDockerSocket(v.HostPath) {
				continue
			}
			if socketGrantCovers(f, name, v) {
				continue
			}
			errs = append(errs, fmt.Errorf("service %q: mounting the Docker socket (%s) is never allowed without a docker_api grant: it grants root on the host [%s]", name, v.HostPath, dockerguard.RuleBindSensitive))
		}
	}
	return errs
}

// dockerAPISocketPath is where a granted service sees its own scoped socket.
const dockerAPISocketPath = "/var/run/docker.sock"

// socketGrantCovers reports whether a docker.sock volume is fine: the service holds a
// docker_api grant, or a rollback is dropping an ungranted one. Only the standard
// host path qualifies, and it is never mounted for real: the reconciler swaps in the grant's own socket.
func socketGrantCovers(f *File, name string, v Volume) bool {
	if !isStandardDockerSocket(v.HostPath) || path.Clean(v.ContainerPath) != dockerAPISocketPath {
		return false
	}
	if _, ok := dockerguard.ProfileFromCaps(f.CapGrants[name]); ok {
		return true
	}
	return f.DropUngrantedCaps
}

func isStandardDockerSocket(hostPath string) bool {
	p := path.Clean(hostPath)
	return p == "/var/run/docker.sock" || p == "/run/docker.sock"
}

func hostAccessViolations(svc Service, granted []string, dropUngranted bool) []string {
	h := svc.HostAccess
	var out []string
	if h.Privileged {
		out = append(out, fmt.Sprintf("privileged: true is not allowed [%s]", dockerguard.RulePrivileged))
	}
	switch strings.ToLower(strings.TrimSpace(h.NetworkMode)) {
	case "host":
		if !dockerguard.HasHostNetworkGrant(granted) && !dropUngranted {
			out = append(out, fmt.Sprintf("network_mode: host is not allowed without a host_network grant; publish a port or add a domain instead [%s]", dockerguard.RuleNetworkHost))
		}
	case "none", "", "bridge":
	default:
		out = append(out, fmt.Sprintf("network_mode: %q is not supported; every service already shares the app network", h.NetworkMode))
	}
	for _, ns := range []struct{ key, value, rule string }{
		{"pid", h.Pid, dockerguard.RuleHostPID},
		{"ipc", h.Ipc, dockerguard.RuleHostIPC},
		{"uts", h.Uts, dockerguard.RuleHostUTS},
		{"userns_mode", h.UsernsMode, dockerguard.RuleHostUserns},
		{"cgroup", h.Cgroup, dockerguard.RuleHostCgroupns},
	} {
		if strings.EqualFold(strings.TrimSpace(ns.value), "host") {
			out = append(out, fmt.Sprintf("%s: host shares a host namespace and is not allowed [%s]", ns.key, ns.rule))
		}
	}
	if denied := undeclaredCaps(h.CapAdd, granted, dropUngranted); len(denied) > 0 {
		out = append(out, fmt.Sprintf("cap_add: %s is not allowed unless the manifest declares it as a capability [%s]", strings.Join(denied, ", "), dockerguard.RuleCapAdd))
	}
	out = append(out, deviceViolations(h.Devices, granted, dropUngranted)...)
	for _, opt := range h.SecurityOpt {
		if weakensSecurity(opt) {
			out = append(out, fmt.Sprintf("security_opt: %q weakens container isolation and is not allowed [%s]", opt, dockerguard.RuleSecurityOpt))
		}
	}
	if len(h.VolumesFrom) > 0 {
		out = append(out, fmt.Sprintf("volumes_from: is not allowed; declare a named volume on both services instead [%s]", dockerguard.RuleVolumesFrom))
	}
	return out
}

// DevicePaths returns the host path of each plain "host[:container[:perms]]" devices entry.
// Entries in any other form, or that remap the node or ask for odd permissions, are left out.
func DevicePaths(devs []any) []string {
	var out []string
	for _, d := range devs {
		s, ok := d.(string)
		if !ok {
			continue
		}
		parts := strings.SplitN(s, ":", 3)
		if len(parts) > 1 && parts[1] != "" && parts[1] != parts[0] {
			continue
		}
		if len(parts) > 2 && strings.Trim(parts[2], "rwm") != "" {
			continue
		}
		out = append(out, parts[0])
	}
	return out
}

func deviceViolations(devs []any, granted []string, dropUngranted bool) []string {
	if len(devs) == 0 {
		return nil
	}
	var out []string
	paths := DevicePaths(devs)
	if len(paths) != len(devs) {
		out = append(out, fmt.Sprintf("devices: only plain host paths without a remap are supported [%s]", dockerguard.RuleDevices))
	}
	held := dockerguard.DevicesFromCaps(granted)
	for _, p := range paths {
		switch {
		case !dockerguard.GrantableDevice(p):
			out = append(out, fmt.Sprintf("devices: %s is not on the device allowlist (/dev/dri, /dev/ttyUSB*, /dev/ttyACM*, /dev/video*) [%s]", p, dockerguard.RuleDevices))
		case !slices.Contains(held, p) && !dropUngranted:
			out = append(out, fmt.Sprintf("devices: %s is not allowed without a devices grant; GPUs go through deploy.resources.reservations.devices [%s]", p, dockerguard.RuleDevices))
		}
	}
	return out
}

// hostNetworkLayout refuses host networking for an app that also needs the project network,
// since a host-network container is on neither side of it.
func hostNetworkLayout(f *File, name string, svc Service) error {
	if !strings.EqualFold(strings.TrimSpace(svc.HostAccess.NetworkMode), "host") || !dockerguard.HasHostNetworkGrant(f.CapGrants[name]) {
		return nil
	}
	if len(svc.HostAccess.CapAdd) > 0 {
		return fmt.Errorf("service %q: network_mode: host cannot be combined with cap_add", name)
	}
	if len(f.Services) > 1 || len(svc.Networks) > 0 {
		return fmt.Errorf("service %q: network_mode: host cannot be combined with other services or networks, the app would need the project network", name)
	}
	return nil
}

// weakensSecurity mirrors the socket proxy: only no-new-privileges is
// purely additive, every other option can loosen a default profile.
func weakensSecurity(opt string) bool {
	o := strings.ToLower(strings.TrimSpace(opt))
	return o != "no-new-privileges" && o != "no-new-privileges:true" && o != "no-new-privileges=true"
}

// isDockerSocket catches a container-runtime socket outside /var/run too,
// which bindmount's forbidden list (path prefixes only) would let through.
func isDockerSocket(hostPath string) bool {
	base := path.Base(strings.TrimRight(hostPath, "/"))
	return base == "docker.sock" || base == "podman.sock" || base == "containerd.sock"
}

// ServiceHostViolations reports what validate rejects for one service's host access and
// bind sources, for callers that cannot fully parse a file yet still must enforce the rules.
func ServiceHostViolations(h HostAccess, hostPaths []string) []string {
	out := hostAccessViolations(Service{HostAccess: h}, nil, false)
	for _, p := range hostPaths {
		if isDockerSocket(p) {
			out = append(out, fmt.Sprintf("mounting the Docker socket (%s) is never allowed: it grants root on the host [%s]", p, dockerguard.RuleBindSensitive))
		}
	}
	return out
}

// undeclaredCaps returns the cap_add values not covered by a grant. A grant is
// only ever built from a policy-checked manifest, and even then must name a Review or Elevated cap.
func undeclaredCaps(capAdd, granted []string, dropUngranted bool) []string {
	var out []string
	for _, c := range capAdd {
		n := dockerguard.NormalizeCap(c)
		known := capInSet(n, dockerguard.ReviewTierCaps) || capInSet(n, dockerguard.GrantableCaps)
		if dropUngranted && known && !capInSet(n, granted) {
			continue
		}
		if !capInSet(n, granted) || (!capInSet(n, dockerguard.ReviewTierCaps) && !capInSet(n, dockerguard.GrantableCaps)) {
			out = append(out, c)
		}
	}
	return out
}

func capInSet(c string, set []string) bool {
	for _, s := range set {
		if dockerguard.NormalizeCap(s) == c {
			return true
		}
	}
	return false
}

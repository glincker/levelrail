package compose

import (
	"fmt"
	"path"
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
		for _, msg := range hostAccessViolations(svc) {
			errs = append(errs, fmt.Errorf("service %q: %s", name, msg))
		}
		for _, v := range svc.Volumes {
			if v.HostPath != "" && isDockerSocket(v.HostPath) {
				errs = append(errs, fmt.Errorf("service %q: mounting the Docker socket (%s) is never allowed: it grants root on the host [%s]", name, v.HostPath, dockerguard.RuleBindSensitive))
			}
		}
	}
	return errs
}

func hostAccessViolations(svc Service) []string {
	h := svc.HostAccess
	var out []string
	if h.Privileged {
		out = append(out, fmt.Sprintf("privileged: true is not allowed [%s]", dockerguard.RulePrivileged))
	}
	switch strings.ToLower(strings.TrimSpace(h.NetworkMode)) {
	case "host":
		out = append(out, fmt.Sprintf("network_mode: host is not allowed; publish a port or add a domain instead [%s]", dockerguard.RuleNetworkHost))
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
	if len(h.CapAdd) > 0 {
		out = append(out, fmt.Sprintf("cap_add: %s is not allowed [%s]", strings.Join(h.CapAdd, ", "), dockerguard.RuleCapAdd))
	}
	if len(h.Devices) > 0 {
		out = append(out, fmt.Sprintf("devices: is not allowed; GPUs go through deploy.resources.reservations.devices [%s]", dockerguard.RuleDevices))
	}
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

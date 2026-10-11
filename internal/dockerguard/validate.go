package dockerguard

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Violation is one rule a request breaks. Reason names fields and host
// paths only, never values such as env vars or credentials.
type Violation struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
}

func violation(rule, format string, args ...any) Violation {
	return Violation{Rule: rule, Reason: fmt.Sprintf(format, args...)}
}

// validateCreate checks a POST /containers/create body against p and the
// declaration Create registered for this container name, if any.
func (p Policy) validateCreate(req container.CreateRequest, grant docker.CreateDeclaration) []Violation {
	hc := req.HostConfig
	if hc == nil {
		return nil
	}
	var out []Violation
	if hc.Privileged {
		out = append(out, violation(RulePrivileged, "HostConfig.Privileged is never allowed"))
	}
	for _, ns := range []struct {
		rule, field, value string
	}{
		{RuleHostPID, "PidMode", string(hc.PidMode)},
		{RuleHostIPC, "IpcMode", string(hc.IpcMode)},
		{RuleHostUTS, "UTSMode", string(hc.UTSMode)},
		{RuleHostUserns, "UsernsMode", string(hc.UsernsMode)},
		{RuleHostCgroupns, "CgroupnsMode", string(hc.CgroupnsMode)},
	} {
		if strings.EqualFold(strings.TrimSpace(ns.value), "host") {
			out = append(out, violation(ns.rule, "HostConfig.%s=host shares a host namespace", ns.field))
		}
	}
	if strings.EqualFold(strings.TrimSpace(string(hc.NetworkMode)), "host") && (!grant.HostNetwork || !p.AllowHostNetwork) {
		out = append(out, violation(RuleNetworkHost, "HostConfig.NetworkMode=host needs a declared host network and %s=true", EnvAllowHostNetwork))
	}
	out = append(out, p.checkCaps(hc.CapAdd, grant)...)
	out = append(out, checkDevices(hc.Resources, grant)...)
	out = append(out, checkSecurityOpt(hc.SecurityOpt)...)
	if hc.MaskedPaths != nil || hc.ReadonlyPaths != nil {
		out = append(out, violation(RuleMaskedPaths, "HostConfig.MaskedPaths/ReadonlyPaths override the default /proc and /sys masking"))
	}
	if len(hc.VolumesFrom) > 0 {
		out = append(out, violation(RuleVolumesFrom, "HostConfig.VolumesFrom inherits another container's mounts"))
	}
	for _, b := range hc.Binds {
		out = append(out, p.checkBindString(b, grant)...)
	}
	for _, m := range hc.Mounts {
		out = append(out, p.checkMount(m, grant)...)
	}
	return out
}

func (p Policy) checkCaps(caps []string, grant docker.CreateDeclaration) []Violation {
	var out []Violation
	for _, raw := range caps {
		c := normalizeCap(raw)
		switch {
		case containsCap(p.AllowedCaps, c):
		case containsCap(GrantableCaps, c) && containsCap(grant.CapAdd, c):
		default:
			out = append(out, violation(RuleCapAdd, "HostConfig.CapAdd %s is outside the hardening profile", c))
		}
	}
	return out
}

func containsCap(list []string, c string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return normalizeCap(x) == c })
}

// gpuDevicePrefixes are the only raw device nodes a GPU-declared container may map.
var gpuDevicePrefixes = []string{"/dev/nvidia", "/dev/dri/"}

func checkDevices(r container.Resources, grant docker.CreateDeclaration) []Violation {
	var out []Violation
	if len(r.DeviceCgroupRules) > 0 {
		out = append(out, violation(RuleDeviceCgroupRules, "DeviceCgroupRules grant raw device access"))
	}
	if len(r.DeviceRequests) > 0 && !grant.GPU {
		out = append(out, violation(RuleDevices, "DeviceRequests need a GPU declared by the app spec"))
	}
	for _, d := range r.Devices {
		gpuNode := slices.ContainsFunc(gpuDevicePrefixes, func(pfx string) bool { return strings.HasPrefix(filepath.Clean(d.PathOnHost), pfx) })
		if !grant.GPU || !gpuNode {
			out = append(out, violation(RuleDevices, "Devices %s is not a GPU node of a GPU-declared app", d.PathOnHost))
		}
	}
	return out
}

func checkSecurityOpt(opts []string) []Violation {
	var out []Violation
	for _, o := range opts {
		key, val, _ := strings.Cut(strings.ToLower(strings.TrimSpace(o)), "=")
		if k, v, ok := strings.Cut(key, ":"); ok && val == "" {
			key, val = k, v
		}
		weakened := false
		switch key {
		case "seccomp", "systempaths":
			weakened = true
		case "apparmor":
			weakened = val == "unconfined"
		case "label":
			weakened = val == "disable" || strings.Contains(val, "spc_t")
		case "no-new-privileges":
			weakened = val == "false"
		}
		if weakened {
			out = append(out, violation(RuleSecurityOpt, "HostConfig.SecurityOpt %q weakens confinement", key))
		}
	}
	return out
}

// checkBindString handles the legacy "src:dst[:opts]" form; a src without
// a leading slash is a named volume.
func (p Policy) checkBindString(b string, grant docker.CreateDeclaration) []Violation {
	src, _, _ := strings.Cut(b, ":")
	if !strings.HasPrefix(src, "/") {
		return nil
	}
	return p.checkHostPath(src, grant)
}

func (p Policy) checkMount(m mount.Mount, grant docker.CreateDeclaration) []Violation {
	switch m.Type {
	case mount.TypeBind:
		return p.checkHostPath(m.Source, grant)
	case mount.TypeVolume:
		if m.VolumeOptions != nil && m.VolumeOptions.DriverConfig != nil && bindDriverOpts(m.VolumeOptions.DriverConfig.Options) {
			return []Violation{violation(RuleVolumeBindDriverOpt, "Mounts volume %s uses bind driver options", m.Source)}
		}
		return nil
	case mount.TypeTmpfs:
		return nil
	default:
		return []Violation{violation(RuleMountType, "Mounts type %q is not allowed", m.Type)}
	}
}

func (p Policy) checkHostPath(hostPath string, grant docker.CreateDeclaration) []Violation {
	if !filepath.IsAbs(hostPath) {
		return []Violation{violation(RuleBindUndeclared, "bind source %q is not absolute", hostPath)}
	}
	if p.sensitiveHostPath(hostPath) {
		return []Violation{violation(RuleBindSensitive, "bind source %s is a protected host path", filepath.Clean(hostPath))}
	}
	clean := filepath.Clean(hostPath)
	if !slices.ContainsFunc(grant.BindPaths, func(d string) bool { return filepath.Clean(d) == clean }) {
		return []Violation{violation(RuleBindUndeclared, "bind source %s was not declared by the app volume model", clean)}
	}
	return nil
}

// bindDriverOpts spots the local driver's type=none,o=bind trick that turns
// a named volume into a bind mount of any host path.
func bindDriverOpts(opts map[string]string) bool {
	for k, v := range opts {
		if !strings.EqualFold(k, "o") {
			continue
		}
		for _, o := range strings.Split(v, ",") {
			switch strings.ToLower(strings.TrimSpace(o)) {
			case "bind", "rbind":
				return true
			}
		}
	}
	return false
}

func validateUpdate(u container.UpdateConfig) []Violation {
	return checkDevices(u.Resources, docker.CreateDeclaration{})
}

func validateExec(e container.ExecOptions) []Violation {
	if e.Privileged {
		return []Violation{violation(RuleExecPrivileged, "exec Privileged is never allowed")}
	}
	return nil
}

func validateVolume(v volume.CreateOptions) []Violation {
	if bindDriverOpts(v.DriverOpts) {
		return []Violation{violation(RuleVolumeBindDriverOpt, "volume %s uses bind driver options", v.Name)}
	}
	return nil
}

package docker

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

// applyRuntimeOptions maps the allowlisted spec fields onto hostConfig after
// hardening, so a user mount wins over the hardening tmpfs and cap_drop only adds.
func applyRuntimeOptions(hc *container.HostConfig, spec ContainerSpec) {
	if spec.Init {
		on := true
		hc.Init = &on
	}
	for _, t := range spec.Tmpfs {
		p, opts, _ := strings.Cut(t, ":")
		if hc.Tmpfs == nil {
			hc.Tmpfs = map[string]string{}
		}
		hc.Tmpfs[p] = opts
	}
	for _, u := range spec.Ulimits {
		hc.Ulimits = append(hc.Ulimits, &container.Ulimit{Name: u.Name, Soft: u.Soft, Hard: u.Hard})
	}
	if len(spec.Sysctls) > 0 {
		hc.Sysctls = make(map[string]string, len(spec.Sysctls))
		for k, v := range spec.Sysctls {
			hc.Sysctls[k] = v
		}
	}
	if len(spec.CapDrop) > 0 {
		hc.CapDrop = mergeCapDrop(hc.CapDrop, spec.CapDrop)
	}
}

func mergeCapDrop(have, extra []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range append(append([]string{}, have...), extra...) {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// configuredStopTimeout lets a stop_timeout set at create extend the caller's
// grace, but never when the caller asked for an immediate kill or no timeout.
func (c *Client) configuredStopTimeout(ctx context.Context, id string, grace time.Duration) time.Duration {
	if grace <= 0 {
		return grace
	}
	info, err := c.cli.ContainerInspect(ctx, id)
	if err != nil || info.Config == nil {
		return grace
	}
	return pickStopTimeout(grace, info.Config.StopTimeout)
}

func pickStopTimeout(grace time.Duration, configured *int) time.Duration {
	if grace <= 0 || configured == nil || *configured < 0 {
		return grace
	}
	return max(grace, time.Duration(*configured)*time.Second)
}

// clampTenantPids keeps an app's pids_limit within the operator's hardening
// cap; with the cap off (rootless) a tenant value is not applied at all.
func (h HardeningConfig) clampTenantPids(hc *container.HostConfig, spec ContainerSpec) {
	if spec.PidsLimit <= 0 || spec.Hardened || hc.PidsLimit == nil {
		return
	}
	// An operator limit of 0 means the host cannot apply one (rootless), in any mode.
	if h.PidsLimit <= 0 {
		hc.PidsLimit = nil
		return
	}
	if h.enforced() && *hc.PidsLimit > h.PidsLimit {
		limit := h.PidsLimit
		hc.PidsLimit = &limit
	}
}

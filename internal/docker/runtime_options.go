package docker

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	units "github.com/docker/go-units"
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
		hc.Ulimits = append(hc.Ulimits, &units.Ulimit{Name: u.Name, Soft: u.Soft, Hard: u.Hard})
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

// configuredStopTimeout prefers a stop_timeout set at create over the caller's grace.
func (c *Client) configuredStopTimeout(ctx context.Context, id string, grace time.Duration) time.Duration {
	info, err := c.cli.ContainerInspect(ctx, id)
	if err != nil || info.Config == nil || info.Config.StopTimeout == nil {
		return grace
	}
	return time.Duration(*info.Config.StopTimeout) * time.Second
}

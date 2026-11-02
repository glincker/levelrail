package docker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// ReadOnlyRootfsLabelKey records an app's explicit read-only root choice
// ("true" or "false"). It rides the agent transport's labels like the shm
// size does, and absence means inherit the fleet default. Must stay under
// spec.ReservedLabelPrefix.
const ReadOnlyRootfsLabelKey = "platform-reserved.read-only-rootfs"

// ReadOnlyTmpfsPaths are the scratch directories a read-only root gets as
// tmpfs, skipped when a volume or bind mount already owns the path.
var ReadOnlyTmpfsPaths = []string{"/tmp", "/run"}

// ReadOnlyRootfsFromLabels reads the explicit choice; set is false when the
// label is absent or invalid, meaning inherit.
func ReadOnlyRootfsFromLabels(labels map[string]string) (on, set bool) {
	v, err := strconv.ParseBool(labels[ReadOnlyRootfsLabelKey])
	if err != nil {
		return false, false
	}
	return v, true
}

// applyReadOnlyRootfs makes the root filesystem read-only when the app asks
// for it, or inherits the fleet default. Hardened specs are already handled.
func (h HardeningConfig) applyReadOnlyRootfs(hc *container.HostConfig, spec ContainerSpec) {
	if spec.Hardened {
		return
	}
	on, set := ReadOnlyRootfsFromLabels(spec.Labels)
	if !set {
		// Inline files are copied into the root before start, which a read-only
		// root refuses, so the fleet default never applies to such a service.
		on = h.ReadOnlyRootfs && len(spec.Files) == 0
	}
	if !on {
		return
	}
	hc.ReadonlyRootfs = true
	size := h.ReadOnlyTmpfsMB
	if size <= 0 {
		size = DefaultReadOnlyTmpfsMB
	}
	opts := fmt.Sprintf("rw,nosuid,nodev,size=%dm", size)
	taken := map[string]bool{}
	for _, v := range spec.Volumes {
		taken[strings.TrimRight(v.ContainerPath, "/")] = true
	}
	for _, b := range spec.BindMounts {
		taken[strings.TrimRight(b.ContainerPath, "/")] = true
	}
	for _, p := range ReadOnlyTmpfsPaths {
		if taken[p] {
			continue
		}
		if hc.Tmpfs == nil {
			hc.Tmpfs = map[string]string{}
		}
		hc.Tmpfs[p] = opts
	}
}

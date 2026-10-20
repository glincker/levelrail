package docker

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
)

// EnvGPUAttach selects how GPUs are attached to containers: "auto"
// (default) prefers CDI when Docker lists the NVIDIA CDI devices and falls
// back to the legacy nvidia runtime, "legacy" never uses CDI.
const EnvGPUAttach = "APP_GPU_ATTACH"

const (
	cdiVendorPrefix = "nvidia.com/gpu="
	cdiCacheTTL     = 30 * time.Second
)

// cdiCache remembers which NVIDIA CDI devices the daemon lists, so a
// burst of container creates asks Docker once.
type cdiCache struct {
	mu      sync.Mutex
	at      time.Time
	devices []string
}

// CDIDevices lists the NVIDIA CDI devices the Docker daemon has
// discovered (for example "nvidia.com/gpu=0" and "nvidia.com/gpu=all").
// The daemon lists none when CDI is unsupported or no spec is installed.
func (c *Client) CDIDevices(ctx context.Context) ([]string, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker: info: %w", err)
	}
	var out []string
	for _, d := range info.DiscoveredDevices {
		if strings.EqualFold(d.Source, "cdi") && strings.HasPrefix(d.ID, cdiVendorPrefix) {
			out = append(out, d.ID)
		}
	}
	return out, nil
}

func (c *Client) cachedCDIDevices(ctx context.Context) []string {
	c.cdi.mu.Lock()
	defer c.cdi.mu.Unlock()
	if time.Since(c.cdi.at) < cdiCacheTTL {
		return c.cdi.devices
	}
	devs, err := c.CDIDevices(ctx)
	if err != nil {
		return nil
	}
	c.cdi.at, c.cdi.devices = time.Now(), devs
	return devs
}

// gpuAttachMode reads EnvGPUAttach; anything unknown means auto.
func gpuAttachMode() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(EnvGPUAttach)), "legacy") {
		return "legacy"
	}
	return "auto"
}

// cdiDeviceRequest maps g onto CDI device names. ok is false unless the
// daemon lists every one of them, so a missing spec falls back to the
// legacy runtime instead of failing the container start.
func cdiDeviceRequest(g GPURequest, discovered []string) (container.DeviceRequest, bool) {
	have := make(map[string]bool, len(discovered))
	for _, d := range discovered {
		have[d] = true
	}
	var ids []string
	switch {
	case len(g.DeviceIDs) > 0:
		for _, id := range g.DeviceIDs {
			ids = append(ids, cdiVendorPrefix+id)
		}
	case g.Count > 0:
		for i := 0; i < g.Count; i++ {
			ids = append(ids, cdiVendorPrefix+strconv.Itoa(i))
		}
	default:
		ids = []string{cdiVendorPrefix + "all"}
	}
	for _, id := range ids {
		if !have[id] {
			return container.DeviceRequest{}, false
		}
	}
	return container.DeviceRequest{Driver: "cdi", DeviceIDs: ids}, true
}

// attachGPU switches hostConfig to a CDI device request when policy and
// the daemon allow it, leaving the legacy request in place otherwise.
func (c *Client) attachGPU(ctx context.Context, hostConfig *container.HostConfig, g GPURequest) {
	if gpuAttachMode() == "legacy" {
		return
	}
	if req, ok := cdiDeviceRequest(g, c.cachedCDIDevices(ctx)); ok {
		hostConfig.DeviceRequests = []container.DeviceRequest{req}
	}
}

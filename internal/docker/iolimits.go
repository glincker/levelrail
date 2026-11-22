package docker

import (
	"github.com/docker/docker/api/types/blkiodev"
	"github.com/docker/docker/api/types/container"
)

// applyIOLimits sets per-device read and write rate caps. Without a resolved
// device the caps are skipped: Docker needs a device path to throttle.
func applyIOLimits(hc *container.HostConfig, r *Resources, device string) {
	if device == "" {
		return
	}
	if r.IOReadBps > 0 {
		hc.BlkioDeviceReadBps = []*blkiodev.ThrottleDevice{{Path: device, Rate: uint64(r.IOReadBps)}}
	}
	if r.IOWriteBps > 0 {
		hc.BlkioDeviceWriteBps = []*blkiodev.ThrottleDevice{{Path: device, Rate: uint64(r.IOWriteBps)}}
	}
}

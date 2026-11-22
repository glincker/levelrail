package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestApplyIOLimits(t *testing.T) {
	hc := &container.HostConfig{}
	applyIOLimits(hc, &Resources{IOReadBps: 1000, IOWriteBps: 2000}, "/dev/sda")
	if len(hc.BlkioDeviceReadBps) != 1 || hc.BlkioDeviceReadBps[0].Rate != 1000 || hc.BlkioDeviceWriteBps[0].Rate != 2000 {
		t.Fatalf("caps not applied: %+v %+v", hc.BlkioDeviceReadBps, hc.BlkioDeviceWriteBps)
	}
	none := &container.HostConfig{}
	applyIOLimits(none, &Resources{IOReadBps: 1000}, "")
	if none.BlkioDeviceReadBps != nil {
		t.Fatal("no device must skip caps")
	}
}

package docker

import (
	"reflect"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestHardenedSpecIgnoresGlobalMode(t *testing.T) {
	for _, mode := range []HardeningMode{HardeningOff, HardeningWarn, HardeningEnforce} {
		spec := ContainerSpec{Name: "p", Hardened: true, CapAdd: []string{"NET_ADMIN"}}
		hc := buildHostConfig(spec, nat.PortMap{})
		HardeningConfig{Mode: mode, ExtraCaps: []string{"CHOWN"}}.apply(hc, spec)
		if !hc.ReadonlyRootfs || !reflect.DeepEqual([]string(hc.CapDrop), []string{"ALL"}) || len(hc.CapAdd) != 0 {
			t.Errorf("mode %s: rootfs=%v drop=%v add=%v, want read-only, drop ALL, nothing added", mode, hc.ReadonlyRootfs, hc.CapDrop, hc.CapAdd)
		}
		if !reflect.DeepEqual(hc.SecurityOpt, []string{"no-new-privileges"}) || hc.PidsLimit == nil || hc.Tmpfs["/tmp"] == "" {
			t.Errorf("mode %s: secopt=%v pids=%v tmpfs=%v", mode, hc.SecurityOpt, hc.PidsLimit, hc.Tmpfs)
		}
	}
}

func TestHardenedCapAddKeepsOnlyListedCaps(t *testing.T) {
	for _, mode := range []HardeningMode{HardeningOff, HardeningEnforce} {
		spec := ContainerSpec{Name: "h", Hardened: true, CapAdd: []string{"NET_ADMIN"}, HardenedCapAdd: []string{"CHOWN"}, NetworkMode: "none"}
		hc := buildHostConfig(spec, nat.PortMap{})
		HardeningConfig{Mode: mode, ExtraCaps: []string{"KILL"}}.apply(hc, spec)
		if !reflect.DeepEqual([]string(hc.CapDrop), []string{"ALL"}) || !reflect.DeepEqual([]string(hc.CapAdd), []string{"CHOWN"}) {
			t.Errorf("mode %s: drop=%v add=%v, want drop ALL and only CHOWN", mode, hc.CapDrop, hc.CapAdd)
		}
		if string(hc.NetworkMode) != "none" {
			t.Errorf("mode %s: network = %q", mode, hc.NetworkMode)
		}
	}
}
